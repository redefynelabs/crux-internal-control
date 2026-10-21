# ERP control plane

The vendor's back office for the ERP. The ERP itself never depends on it.

**Today:** demo requests, contact messages, demo instances that live for one hour, the leads they produce, a small
sales CRM (owners, follow-up tasks, email), recorded sales and revenue analytics (MRR, ARR, churn, funnel). The admin UI
is in `web/`. Tenants and billing come next.

## What it does

1. The marketing site posts `POST /v1/demo-requests` (name, email, optional phone and company).
2. The control plane checks limits (per email and per address in 24 h, total live demos, optional Turnstile captcha,
   a honeypot field), records or refreshes the **lead**, and creates a **demo**:
   - `CREATE DATABASE demo_<slug> TEMPLATE erp_demo_seed` (about half a second; see the ERP's `ops/demo/build-template.sh`)
   - a private Docker network with a `server` container (`DEPLOYMENT_MODE=demo`, its own Redis prefix, secrets, and
     `MIGRATE_ON_START=true`) and a `web` container, memory and CPU capped, routed by Traefik at `<slug>.<domain>`
3. It answers with the address, the demo login and the expiry time. The login is made for that demo alone: the
   visitor's own email as the administrator, and a new 14-character password (`DEMO_ADMIN_EMAIL` and
   `DEMO_ADMIN_PASSWORD` for the demo server, which applies them at start). The password is shown once and is not kept
   by the control plane.
4. A reaper runs every minute. It removes demos past their time (containers, network, database), failed starts, and
   any container no demo row accounts for, then moves the lead to `demo_ended` (never pulling back a lead you already
   worked).

## API

Public: `POST /v1/demo-requests`, `POST /v1/contact-requests` (a message to sales, no demo), `GET /healthz`. Both forms accept an `attribution` object (utm_*, referrer, landing) that is kept on the lead's history, a `website` honeypot field and an optional `captcha_token`.

Called by ERP installations (authenticated by the installation's credential, not a session; see "Entitlements"): `POST /v1/access-requests`, `GET /v1/entitlements/current`.

Back office (`Authorization: Bearer <session token from POST /v1/auth/login>`; `staff` = admin or sales, `admin` = admin only):

| Area | Routes |
|---|---|
| Session | `POST /v1/auth/login`, `POST /v1/auth/logout`, `GET /v1/auth/invite/{token}`, `POST /v1/auth/accept-invite`, `GET /v1/me`, `POST /v1/me/password` |
| Team | `GET /v1/users` (staff); `POST /v1/users`, `PATCH /v1/users/{id}`, `POST /v1/users/{id}/invite` (admin). The last active admin cannot be removed or demoted (409) |
| Leads | `GET/POST /v1/leads`, `GET/PATCH /v1/leads/{id}`, `POST /v1/leads/{id}/notes`, `.../calls`, `.../emails`, `.../tasks`, `GET /v1/stats` |
| Tasks | `GET /v1/tasks`, `PATCH/DELETE /v1/tasks/{id}` |
| Sales | `GET/POST /v1/sales`, `GET/PATCH /v1/sales/{id}`, `DELETE /v1/sales/{id}` (admin) |
| Analytics | `GET /v1/analytics?months=12` |
| Settings | `GET /v1/templates`, `PUT /v1/templates/{id}`, `GET/PUT /v1/settings/email`, `POST /v1/settings/email/test`, `GET/PUT /v1/settings/general` (all admin) |
| Entitlements | `GET /v1/entitlements/catalog`, `GET /v1/installations`, `GET /v1/installations/{id}`, `GET /v1/installations/{id}/entitlements/{version\|latest}/file`, `GET /v1/access-requests?status=` (staff); `POST /v1/installations`, `PATCH /v1/installations/{id}`, `GET /v1/installations/{id}/credential`, `POST .../credential/rotate`, `POST /v1/installations/{id}/entitlements` (issue), `POST /v1/installations/{id}/requests/{request}/decline` (admin) |
| Demos | `GET /v1/demos?status=&limit=`, `POST /v1/demos/{id}/extend` (`minutes`, a demo never lasts over 24 h), `DELETE /v1/demos/{id}` (end it now) |

Lead statuses: `new`, `demo_active`, `demo_ended`, `contacted`, `qualified`, `won`, `lost`.

## The team, leads and email

People sign in to the admin app with an email and password. Two roles: **admin** (everything, including team, templates and
settings) and **sales** (leads, follow-up tasks, email and demos). Admins invite people from **Team**; the invitee sets
their own password from a link. There is no shared token. The first admin comes from `CP_ADMIN_EMAIL` and
`CP_ADMIN_PASSWORD` on the first start (the server refuses to start with nobody to sign in). A forgotten password is reset
on the server: `CP_NEW_PASSWORD=... controlplane reset-password someone@example.com`.

A lead has an owner, the plan they asked about (from the pricing page), a pipeline stage (`new → contacted → qualified →
proposal → won/lost`, with demo started/ended recorded alongside), an expected value, and follow-up **tasks**. Every
note, call, email, status change and owner change lands in one timeline with who did it.

When a lead arrives (demo request or contact form) the control plane can assign an owner (round robin), create the first
follow-up task, and email the visitor an acknowledgement; after a demo ends it can email a follow-up. Each is a switch in
**Settings → General**.

**Settings → Email** takes any SMTP server (Zoho, Google Workspace, SES, Resend, Brevo). The password is stored encrypted
(`CP_SECRETS_KEY`) and never shown again. Email goes through a queue, retried up to three times; failures show on the lead
and on the overview. Templates (with `{{name}}`, `{{first_name}}`, `{{company}}`, `{{demo_url}}`, `{{owner}}`,
`{{signature}}`) are edited under **Templates**. Replies go to the sender's own inbox; they are not read back yet.

## Sales and analytics

A **sale** records what was sold: customer, plan, billing (`monthly`, `yearly` or `one_time`), amount (whole currency
units), start date, owner, and an optional link to the lead. Creating a sale linked to a lead moves the lead to `won`.
Cancelling a sale (`status: cancelled`, with a reason) stamps `ended_on` and takes it out of recurring revenue;
setting it `active` again clears both. Only admins delete sales.

**Analytics** (`GET /v1/analytics`) is computed from sales, leads and tasks:

- **MRR**: monthly sales plus one twelfth of each yearly sale, active ones only. **ARR** = 12 × MRR. One-time sales never
  count toward either; they are summed as `one_time_12m`.
- Customers, ARPA, new and churned MRR over 30 days, churn rate over 90 days, win rate, average deal (yearly value),
  average days to close, pipeline value and a **weighted pipeline** (new 5%, demo 10%, contacted 20%, qualified 40%,
  proposal 70%).
- A monthly series (MRR, new and churned MRR, customers, one-time revenue, new leads, wins, demos), the lead funnel,
  leads and wins by source, MRR by plan, and a per-salesperson table with task counts.

The admin app shows these under **Sales** and **Analytics**. The leads board moves a lead between stages by drag and
drop (dropping on *won* opens the sale form); the tasks board drags a task to a day to reschedule it or into *done*.

Tickets are not part of the control plane; they will be a separate app.

Deploying to a real server: see `deploy/` (one server for the control plane and the demos, with HTTPS).

## Entitlements

What each ERP installation may use (apps, features, the words for records, the starting template) is a small file this service signs with an Ed25519 key, in the format of `erp-software/docs/entitlements.md`. The ERP holds only the public key and does what the file says.

- **Key.** `controlplane keygen <file>` (or `CP_ENTITLEMENT_KEY_FILE`) writes the private key with mode 0600, refuses to replace one, and prints `ENTITLEMENT_PUBLIC_KEY=...` for every installation. `controlplane public-key <file>` shows it again. The service refuses to start when the key file can be read by others. Keep the key out of the repository, the database and backups that leave the server. Losing it means giving every installation a new public key; there is no second accepted key yet.
- **Customers.** Entitlements > Add customer makes an installation; its page shows the three settings to put in the ERP's environment (`INSTALLATION_ID`, `INSTALLATION_SECRET`, `ENTITLEMENT_PUBLIC_KEY`, plus `CONTROL_PLANE_URL`). The secret is derived from `CP_SECRETS_KEY`, the installation and a generation number, so it is not stored; "Replace secret" raises the number and the old one stops working. Changing `CP_SECRETS_KEY` replaces every secret.
- **Issuing.** The editor builds the next version (apps, features, template, record words, expiry), signs it and keeps it. Versions only go up. The file can be downloaded for a first setup (`seed --entitlement`, or the setup screen), or the ERP fetches it every 6 hours from `GET /v1/entitlements/current`.
- **Requests.** The ERP posts what a customer asks for to `POST /v1/access-requests` (a retry with the same id changes nothing). **Approve** opens the editor with the request ticked and its app and features switched on; issuing marks it approved. **Decline** needs a note and signs a new version at once (when one exists) that carries the note, which the customer sees.
- **Calls from the ERP** carry `X-Installation-Id`, `X-Timestamp` and `X-Signature` (HMAC-SHA256 of id, timestamp, method, path and body hash); a timestamp more than 5 minutes off is refused.
- **Demos.** Each demo is recorded as an installation and signed an entitlement with every app and feature, under `CP_DEMO_ENTITLEMENT_TEMPLATE`. Demo servers do not fetch or apply it yet: they run on the copied database.

## Run it

```bash
cp .env.example .env     # fill it in
set -a; . ./.env; set +a
go run ./cmd/controlplane
```

The demo host needs: Docker, a Postgres with the `erp_demo_seed` template and an `erp_demo` role that owns the demo
databases (`CREATE ROLE erp_demo LOGIN PASSWORD '...'`; grant it to the admin role), a Redis, Traefik with a wildcard
certificate for `*.demo.<domain>` (DNS wildcard to the host), and the two Docker networks named in `.env`
(`erp-data` for Postgres and Redis, `erp-proxy` for Traefik).

## Try it on a laptop

Runs the real flow (form, provisioning, a demo opening at `http://<slug>.demo.localhost`) with Docker Desktop.

```bash
make dev-up                                   # Postgres :5434, Redis, Traefik :80
psql postgres://postgres:postgres@localhost:5434/postgres -c "CREATE ROLE erp_demo LOGIN PASSWORD 'local-dev'" -c "GRANT erp_demo TO postgres"
make dev-template                             # the sample company
# images: docker build -t local/erp-web:local ../erp-software/web
#         docker build -t local/erp-server:local ../erp-software/server
```

In `.env`: `CP_PG_ADMIN_URL=postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable`, `CP_PG_HOST=postgres:5432`,
`CP_PG_PASSWORD=local-dev`, `CP_REDIS_URL=redis://redis:6379/1`, `CP_DEMO_DOMAIN=demo.localhost`, `CP_DEMO_SCHEME=http`,
`CP_TRAEFIK_ENTRYPOINT=web`, `CP_TRAEFIK_CERT_RESOLVER=none`, and the two image names above.

## Test

```bash
CP_TEST_DATABASE_URL='postgres://user@localhost:5432/postgres?sslmode=disable' go test ./...
```

Without the variable the tests that need Postgres are skipped. Run them with it set before trusting a change: the
team, lead, sales and analytics tests all live behind it. No test needs Docker: the container backend is tested
against a fake command runner.

## Security notes

- The container backend passes the demo's secrets through a mode-0600 temporary file, never on a command line.
- Names put into SQL (`demo_<slug>`) are generated here and checked against a strict pattern anyway.
- The process needs the Docker socket, which is root-equivalent: keep this host for demos only.
- The back-office API is bearer-token only and is never opened to browser origins (CORS applies to the demo endpoint
  alone).


## Licensing, activation and releases

- **Customers** page: every installation with a worked-out state (awaiting activation, active, out of date, unreachable, expired, suspended, revoked), licence, maintenance (AMC) end, the ERP version it runs and when it last checked in.
- **Editor:** tick business outcomes (CRM, Sales and orders, Projects, People and HR, Invoices and expenses, Inventory); the apps and features behind them are switched on, dependencies included. Fine-tune below. Licence is a subscription (ends on a date) or lifetime; "Updates included until" is the AMC.
- **Activation code:** `POST /v1/installations/{id}/activation` (admin) makes a one-time code, valid 7 days, stored only as a hash. `POST /v1/activate {code}` (public, rate limited) returns the installation id, secret, public key and first file.
- **Check-in:** `POST /v1/sync` (signed like the other installation calls) takes `applied_version`, `server_version`, `server_build` and answers with the key status, the newest file if the installation lacks it, and the releases its AMC covers. `GET /v1/entitlements/current` still works for older servers and counts as a check.
- **Access:** suspend (read-only, file carries it), resume, revoke (nothing more is sent). Nothing is deleted.
- **Releases:** publish a version with images by digest; the manifest is signed with the entitlement key. Beta is never offered; withdrawn releases stop being offered. The ERP's release workflow does this by itself: it calls `POST /v1/ci/releases` with `CP_RELEASE_TOKEN` (a bearer token that can do only this; sending a version again changes nothing). See `erp-software/docs/Hosting.md`, "Releasing to the demos and to customers".
- **Activity:** an audit trail of who changed what.

Needs an ERP release that reads `license`, `updates_until` and `suspended` before such files are issued to it. The ERP's activation client and 15 minute check are not built yet (`erp-software/openspec/changes/add-activation`).

## One-line install for customers

Set `CP_PUBLIC_URL` (this service's public address), `CP_INSTALL_REGISTRY` (for example `ghcr.io/your-org`) and, for private images, a read-only `CP_INSTALL_REGISTRY_USER` / `CP_INSTALL_REGISTRY_TOKEN`. Then each customer page, once you make an activation code, shows a ready command:

    curl -fsSL https://api.example.com/install | sudo bash -s -- --code ACT-... --domain erp.customer.com

`GET /install` serves the script (public, holds no secret), `GET /v1/install/ip` tells it which address it is running from, and `POST /v1/install/start {code}` (public, rate limited) returns the registry, its read-only login and the newest release the customer's maintenance covers, **without using the code up**: the customer's setup does that. The installer unpacks the compose file and scripts from the image itself (`server install-files`), so a release only needs its images published. Starting an install is recorded under Activity.

The registry login is one shared read-only token, handed to anyone holding a valid code. Per-customer tokens that stop at the end of the maintenance are not built.
