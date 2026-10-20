# demo-provisioning Specification

## Purpose

Gives each prospect a private, time-limited copy of the ERP and removes it afterwards.

## Requirements

### Requirement: Provisioning
A demo request SHALL create the database `demo_<slug>` from `CP_DEMO_TEMPLATE`, a private Docker network, and a `server` and `web` container (capped by `CP_DEMO_MEMORY` and `CP_DEMO_CPUS`) routed by Traefik at `<slug>.<CP_DEMO_DOMAIN>`. The server container SHALL get `DEPLOYMENT_MODE=demo`, `DEMO_EXPIRES_AT`, its own Redis prefix and secrets, `MIGRATE_ON_START=true`, and the `DEMO_AI_*` values from `CP_DEMO_AI_*`. The response SHALL carry the address, login and expiry. Secrets SHALL pass through a mode-0600 temporary file, never a command line.

### Requirement: A sign-in for each visitor
Each demo SHALL be given the requesting visitor's email (lowercase) as its administrator and a new random password of 14 characters without look-alike characters (`0 O 1 l I`), passed to the demo server as `DEMO_ADMIN_EMAIL` and `DEMO_ADMIN_PASSWORD`. The answer to the request SHALL carry them as `login_email` and `login_password`. The control plane SHALL NOT store the password.

#### Scenario: Two visitors
- **WHEN** two visitors each request a demo
- **THEN** each demo has its own administrator email and password, and neither opens the other's demo

### Requirement: A waiting line when every room is busy
When `CP_MAX_DEMOS` demos are live, or others already wait, a valid request SHALL be saved (lead, history and a `queued` demo row with its choices) and answered `202 {queued: true, position}` without building anything. Nobody SHALL jump the line. A dispatcher (every 10 s) SHALL start the longest waiting request when a room is free; its hour starts then, and the visitor SHALL be emailed the address, login and expiry. The request SHALL be turned away with `503 at_capacity` instead when email cannot be sent (the login could not be delivered), when `CP_DEMO_QUEUE_MAX` (default 30, 0 turns waiting off) are already waiting, unless the visitor already waits, in which case they keep their place. A start that fails SHALL go back in line, keeping its place, and be given up with an apology after three tries. A request still waiting after `CP_DEMO_QUEUE_WAIT` (default 24h) SHALL be given up with an apology. Staff MAY remove a waiting visitor; nothing is emailed. Per-email and per-IP limits count queued requests.

#### Scenario: Last room taken
- **WHEN** a visitor asks for a demo while every room is busy and email works
- **THEN** they get a place in line, a lead exists, and when a room frees up the demo starts and its login is emailed

### Requirement: Lifetime
A demo SHALL live `CP_DEMO_TTL` (default 1h) and SHALL NOT be extended past 24 hours in all. Staff MAY extend (`minutes`) or end it now.

### Requirement: Reaper
Every minute the control plane SHALL remove demos past their time, failed starts, and any container no demo row accounts for, including containers, network and database, then move the lead to `demo_ended`.

#### Scenario: Expired demo
- **WHEN** a demo passes its expiry
- **THEN** within a minute its containers, network and database are gone and its lead is `demo_ended`, unless the lead was already worked
