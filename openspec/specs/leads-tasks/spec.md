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

### Requirement: Email
Sending email to a lead SHALL use the saved SMTP settings through a queue, retried up to three times, and failures SHALL show on the lead and the overview. The SMTP password SHALL be stored encrypted with `CP_SECRETS_KEY` and never shown. Templates SHALL support `{{name}}`, `{{first_name}}`, `{{company}}`, `{{demo_url}}`, `{{owner}}`, `{{signature}}`.
