# leads-tasks Specification

## Purpose

The vendor's small CRM: where each lead stands, who owns it, what was said, and what to do next.

## Requirements

### Requirement: Pipeline
A lead SHALL be `new`, `demo_active`, `demo_ended`, `contacted`, `qualified`, `proposal`, `won` or `lost`. Changing status, owner, notes, calls and emails SHALL each add a timeline event with the person who did it.

### Requirement: Board
The admin app SHALL show leads as a board; dragging a card SHALL change its stage. Dropping on *won* SHALL open the sale form instead of changing the stage directly.

### Requirement: Tasks
A task belongs to a lead and has a title, a kind (`call`, `email` or `other`), a due time and an assignee, and is open until it has a done time. The tasks board SHALL let dragging to a day reschedule it and dragging to *done* complete it. **New task** SHALL create one. Overdue and open counts SHALL feed the analytics team table.

### Requirement: Designed emails
An email template SHALL be plain text or a designed (HTML) email. A designed template holds the message content only (headings, paragraphs, lists, links and the `[[button: Label | URL]]`, `[[steps: a | b]]` and `[[details: Label = value | ...]]` shortcodes); when sent it SHALL be wrapped in the Crux layout (logo served at `/v1/brand/logo.png`, a card, a footer) and sent as `multipart/alternative` with an automatic plain-text copy, which is also what the lead's history shows. Placeholder values SHALL be HTML-escaped in the HTML part and used as typed in the text part, and a button whose address is empty or not http(s)/mailto SHALL be left out. The template editor (admins) SHALL show a live preview with sample values and SHALL send a test to the signed-in admin. Choosing a designed template in the lead's composer SHALL send the designed email with the person's typed text as an optional note above it. The messages the control plane sends by itself (`demo_ready`, `demo_queue_sorry`) SHALL be editable templates, with built-in plain text as the fallback.

#### Scenario: Designed follow-up
- **WHEN** sales sends the designed "Still interested?" template with a personal note
- **THEN** the lead receives an email with the Crux logo, the note above the message, one button, and a plain-text alternative

### Requirement: Email
Sending email to a lead SHALL use the saved SMTP settings through a queue, retried up to three times, and failures SHALL show on the lead and the overview. The SMTP password SHALL be stored encrypted with `CP_SECRETS_KEY` and never shown. Templates SHALL support `{{name}}`, `{{first_name}}`, `{{company}}`, `{{demo_url}}`, `{{owner}}`, `{{signature}}`.

### Requirement: Deleting a lead
An admin (not a sales person) SHALL be able to delete a lead (`DELETE /v1/leads/{id}`, the **Delete lead** button on the lead page, after a confirmation). Deleting SHALL end any running or waiting demo first, then remove the lead with its history, tasks, emails, demos and the demo installation records, and SHALL write `lead.deleted` to the audit log. A lead that has a sale or a customer (non-demo) installation SHALL be refused with `409 has_records`, so revenue and licences never lose their owner.

#### Scenario: Test lead
- **WHEN** an admin deletes a lead that has a running demo
- **THEN** the demo's containers and database are removed, the lead and its records are gone, and the deletion is in the audit log
