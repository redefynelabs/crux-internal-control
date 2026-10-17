# lead-intake Specification

## Purpose

Turns website demo requests and contact messages into owned, followed-up leads without letting bots or repeat requests flood the system.

## Requirements

### Requirement: Public forms
`POST /v1/demo-requests` and `POST /v1/contact-requests` SHALL accept name, email, phone, company, plan, team size, message, an `attribution` object (utm_*, referrer, landing), a `website` honeypot and an optional `captcha_token`. A filled honeypot SHALL look like success and create nothing. Only `CP_ALLOWED_ORIGINS` may call them from a browser.

### Requirement: One lead per email
A request for an email already known SHALL refresh that lead and add a history event, not create a second lead. A lead already worked SHALL NOT be pulled back to an earlier stage by later events.

### Requirement: Limits
A demo request SHALL be refused (429) past `CP_DEMO_PER_EMAIL_24H`, `CP_DEMO_PER_IP_24H` or `CP_MAX_DEMOS` live demos. With `CP_TURNSTILE_SECRET` set a valid captcha SHALL be required.

### Requirement: Automatic handling
Per Settings → General, a new lead MAY get an owner by round robin, a first follow-up task due after `followup_hours`, an acknowledgement email, and a follow-up email after its demo ends. Round robin SHALL rotate between active salespeople by who was given a lead longest ago, and SHALL use admins only when no salesperson is active.

#### Scenario: Pricing request
- **WHEN** a contact request names the business plan and round robin is on with one active salesperson
- **THEN** the lead carries the plan, that person as owner and a follow-up time, and one acknowledgement email is queued
