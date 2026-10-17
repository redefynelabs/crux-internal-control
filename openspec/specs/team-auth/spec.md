# team-auth Specification

## Purpose

Lets the vendor's own people sign in to the back office with a role, with no shared secret.

## Requirements

### Requirement: Email and password sign-in
`POST /v1/auth/login` SHALL exchange an email and password of an active user for a session token that expires after 12 hours. `POST /v1/auth/logout` SHALL end it at once. The admin app SHALL keep the token in an httpOnly, SameSite=Strict cookie, `Secure` unless `ADMIN_INSECURE_COOKIE=true`. There SHALL be no shared admin token.

#### Scenario: Signed out
- **WHEN** a session is logged out and reused
- **THEN** the API answers 401

### Requirement: Roles
A user SHALL be `admin` or `sales`. Admins SHALL do everything, including team, templates, settings and deleting sales. Sales SHALL work leads, tasks, sales, email and demos. Admin-only routes SHALL answer 403 to sales.

### Requirement: Invitations
An admin SHALL invite a person by name, email and role. The invitee SHALL set their own password from a single-use link (`/invite/{token}`). With `CP_ADMIN_URL` unset the link is shown on screen only.

### Requirement: First admin and recovery
On first start with no users the control plane SHALL create the admin from `CP_ADMIN_EMAIL` and `CP_ADMIN_PASSWORD` (at least 10 characters) and SHALL refuse to start without them. `CP_NEW_PASSWORD=... controlplane reset-password <email>` SHALL set a new password on the server.

### Requirement: Last admin
The last active admin SHALL NOT be deactivated or demoted (409).

#### Scenario: Two admins
- **WHEN** one of two admins is deactivated
- **THEN** it succeeds, and deactivating the remaining one answers 409
