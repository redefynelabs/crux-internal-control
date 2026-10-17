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

### Requirement: Lifetime
A demo SHALL live `CP_DEMO_TTL` (default 1h) and SHALL NOT be extended past 24 hours in all. Staff MAY extend (`minutes`) or end it now.

### Requirement: Reaper
Every minute the control plane SHALL remove demos past their time, failed starts, and any container no demo row accounts for, including containers, network and database, then move the lead to `demo_ended`.

#### Scenario: Expired demo
- **WHEN** a demo passes its expiry
- **THEN** within a minute its containers, network and database are gone and its lead is `demo_ended`, unless the lead was already worked
