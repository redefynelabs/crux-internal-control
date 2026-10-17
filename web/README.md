# Control plane admin

The back-office UI for the control plane: leads, tasks, sales, analytics, team and settings, and a monitor for running demos. It uses the same
design tokens and typeface as the ERP.

It holds no data. Every page reads the control plane's back-office API (`CONTROL_PLANE_URL`).

## Sign-in

You sign in with your email and password (an admin invites you from Team). The control plane returns a session token,
kept in an httpOnly, SameSite=Strict cookie for 12 hours, so browser scripts never see it. The cookie is `Secure` unless
`ADMIN_INSECURE_COOKIE=true` (a private network without HTTPS). Put this app behind HTTPS and, ideally, an IP allow list
or an access proxy; it is a back office, not a public page.

## Run

```bash
cp .env.example .env.local
npm install
npm run dev        # http://localhost:3100
```

`npm run build && npm start` for production, or `docker build -t erp-control-plane-web .`
(`docker run -p 3100:3100 -e CONTROL_PLANE_URL=http://control-plane:8090 erp-control-plane-web`).

## Pages

- `/` overview: counts, pipeline, latest leads, running demos
- `/leads` filter by status, search; `/leads/{id}` status, notes, history, where the lead came from, their demos
- `/leads` also has a board view: drag a card to change its stage; dropping on *won* opens the sale form
- `/tasks` follow-up tasks as a board: drag to a day to reschedule, or to *done*; **New task** adds one
- `/sales`, `/sales/new`, `/sales/{id}` recorded sales: plan, billing, amount, cancel and reactivate
- `/analytics` MRR, ARR, churn, funnel, plans, sources and the team table
- `/team` invite people, change roles, deactivate (admin); `/templates` email templates (admin)
- `/settings` your account, email (SMTP) and general switches (admin)
- `/demos` running demos with time left, extend, end now; recently ended
