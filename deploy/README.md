# One server for the control plane and the demos

Everything runs on a single Linux server with Docker: Traefik (HTTPS), Postgres, Redis, the control plane API, its
admin app, and every demo the control plane starts. The control plane is light (well under 0.5 GB), so it shares the
server with the demos instead of needing a machine of its own.

```
internet ──► Traefik :443 ──► admin.<domain>   admin app
                          ├─► api.<domain>     control plane (the marketing site calls it)
                          └─► <slug>.demo.<domain>  one demo each (server + web containers, own database)
Postgres and Redis are on a private Docker network; Postgres is also on 127.0.0.1 for this server's own tools.
```

## What it needs

| Server | Live demos (`CP_MAX_DEMOS`) |
|---|---|
| 2 vCPU, 4 GB | about 6 |
| 4 vCPU, 8 GB | about 16 |

About 1.3 GB goes to the system, Docker, Postgres, Redis, Traefik and the control plane; each live demo takes about
0.25 to 0.35 GB. Measure with `docker stats --no-stream` after starting three demos, and set `CP_MAX_DEMOS` below the
number that fits. 50 GB of disk is plenty. Open ports 80 and 443 to everyone and 22 to you only.

A domain whose DNS you control, with A records for `admin`, `api` and `*.demo` pointing at the server.

## Install

```bash
git clone <repo> /opt/erp-host && cd /opt/erp-host/erp-control-plane/deploy
cp .env.example .env && cp acme.env.example acme.env && chmod 600 .env acme.env   # fill both in
sed -i "s/^DOCKER_GID=.*/DOCKER_GID=$(getent group docker | cut -d: -f3)/" .env
docker compose up -d
```

1. **The demo template**, once per release (it needs the release's `erp-demo-tools` image and `psql` on this server):
   ```bash
   source .env
   ERP_DEMO_TOOLS_IMAGE=$ERP_DEMO_TOOLS_IMAGE ../../erp-software/ops/demo/build-template.sh \
     "postgres://postgres:$PG_ADMIN_PASSWORD@127.0.0.1:5432/postgres?sslmode=disable"
   ```
   Rebuild it whenever a release adds migrations. Demos already running keep the data they started with.
2. Sign in at `https://admin.<domain>` with `CP_ADMIN_EMAIL` and `CP_ADMIN_PASSWORD`, then set up email under
   Settings → Email.
3. On the marketing site set `CONTROL_PLANE_URL=https://api.<domain>`.

## Check it works

Request a demo from the site. A lead appears in the admin app; the demo opens at its own address and signs in with the
requester's email and the password shown to them; an hour later its containers, network and database are gone.

## Run it

- **Upgrade:** change the image tags in `.env`, `docker compose pull && docker compose up -d`, rebuild the template if
  the release has migrations.
- **Back up** the control plane's database daily with `backup.sh` and copy the file off the server.
- **Entitlement key.** Once, make the signing key where the compose file expects it, owned by the control plane's user:
  `mkdir keys && sudo chown 10001 keys && docker compose run --rm -v "$PWD/keys:/keys" controlplane keygen /keys/entitlement.key`
  It prints `ENTITLEMENT_PUBLIC_KEY=...` for the ERP installations. Back the key file up separately, somewhere safe and
  offline: it is not in the database dump, and without it no installation accepts a new file.
- **See what is running:** `docker ps --filter label=cp.demo` lists the demos; the admin app's Demos page shows the
  same with time left.
- **After a reboot** the demos do not come back (they never restart on their own); the control plane removes their
  leftovers within a minute and the visitors can request again.

## Risks to know

- The control plane uses the Docker socket, which is the same as root on this server. Demo containers run read-only
  where they can, with no capabilities, no new privileges, a memory, CPU and process limit, and their own network, but
  keep this server for this job only.
- The admin app is on the internet behind a sign-in. Add an IP allow list in front of it if you can.
- Postgres has no public port. Do not publish it.
