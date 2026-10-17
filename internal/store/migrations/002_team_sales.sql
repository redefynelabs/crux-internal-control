-- Team: people who sign in, their sessions, and the invite they accept to set a password.
CREATE TABLE users (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT        NOT NULL,
    email              TEXT        NOT NULL,
    role               TEXT        NOT NULL DEFAULT 'sales',
    password_hash      TEXT        NOT NULL DEFAULT '',
    active             BOOLEAN     NOT NULL DEFAULT true,
    invite_hash        TEXT        NOT NULL DEFAULT '',
    invite_expires_at  TIMESTAMPTZ,
    last_login_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'sales'))
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
    token_hash  TEXT        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- Leads gain an owner, what they want, and what the deal is worth.
ALTER TABLE leads
    ADD COLUMN owner_id        UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN interest        TEXT NOT NULL DEFAULT '',
    ADD COLUMN plan_interest   TEXT NOT NULL DEFAULT '',
    ADD COLUMN team_size       TEXT NOT NULL DEFAULT '',
    ADD COLUMN expected_value  BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN lost_reason     TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_contact_at TIMESTAMPTZ;
ALTER TABLE leads DROP CONSTRAINT leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check
    CHECK (status IN ('new', 'demo_active', 'demo_ended', 'contacted', 'qualified', 'proposal', 'won', 'lost'));
CREATE INDEX leads_owner_idx ON leads (owner_id, status);

-- Who did what on a lead.
ALTER TABLE lead_events
    ADD COLUMN actor_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN actor_name TEXT NOT NULL DEFAULT '';

-- Follow-ups: things a person must do for a lead, by a date.
CREATE TABLE lead_tasks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id      UUID        NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    title        TEXT        NOT NULL,
    kind         TEXT        NOT NULL DEFAULT 'other',
    due_at       TIMESTAMPTZ NOT NULL,
    assignee_id  UUID        REFERENCES users (id) ON DELETE SET NULL,
    done_at      TIMESTAMPTZ,
    created_by   UUID        REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT lead_tasks_kind_check CHECK (kind IN ('call', 'email', 'other'))
);
CREATE INDEX lead_tasks_lead_idx ON lead_tasks (lead_id);
CREATE INDEX lead_tasks_open_idx ON lead_tasks (assignee_id, due_at) WHERE done_at IS NULL;

-- Mail waiting to go out, and a record of what went.
CREATE TABLE emails (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id     UUID        REFERENCES leads (id) ON DELETE CASCADE,
    sent_by     UUID        REFERENCES users (id) ON DELETE SET NULL,
    to_email    TEXT        NOT NULL,
    subject     TEXT        NOT NULL,
    body        TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'queued',
    error       TEXT        NOT NULL DEFAULT '',
    attempts    INT         NOT NULL DEFAULT 0,
    send_after  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at     TIMESTAMPTZ,
    CONSTRAINT emails_status_check CHECK (status IN ('queued', 'sent', 'failed'))
);
CREATE INDEX emails_due_idx ON emails (send_after) WHERE status = 'queued';
CREATE INDEX emails_lead_idx ON emails (lead_id, created_at DESC);

CREATE TABLE email_templates (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    key         TEXT        NOT NULL UNIQUE,
    name        TEXT        NOT NULL,
    subject     TEXT        NOT NULL,
    body        TEXT        NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Settings kept as one JSON document per area ("email", "general").
CREATE TABLE settings (
    key         TEXT        PRIMARY KEY,
    value       JSONB       NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO email_templates (key, name, subject, body) VALUES
('contact_received', 'Thanks for getting in touch', 'We got your message, {{first_name}}',
E'Hi {{first_name}},\n\nThanks for writing to us. We have your message and someone from the team will come back to you within one working day.\n\nIf you would like to see the product while you wait, you can start a private demo any time, no card needed.\n\n{{signature}}'),
('demo_followup', 'After the demo', 'How was the demo, {{first_name}}?',
E'Hi {{first_name}},\n\nI hope you had a chance to look around the demo and try asking Mei a question. If you would like me to show you around for 20 minutes, or talk through pricing for {{company}}, just reply to this email.\n\n{{signature}}'),
('cloud_plan', 'Cloud plan details', 'Running {{company}} on our cloud plan',
E'Hi {{first_name}},\n\nThanks for your interest in the cloud plan. We host and look after it for you: one flat price by size, every feature, unlimited users, daily backups, and your own separate instance.\n\nTell me roughly how many people you have and I will suggest the right size and send a quote.\n\n{{signature}}'),
('proposal_sent', 'Proposal sent', 'Your proposal for {{company}}',
E'Hi {{first_name}},\n\nAs discussed, here is the proposal for {{company}}. Have a look and tell me what you would change. I am happy to jump on a call to go through it.\n\n{{signature}}'),
('check_in', 'Still interested?', 'Still thinking about it, {{first_name}}?',
E'Hi {{first_name}},\n\nI wanted to check in. Is there anything I can answer, or anything blocking you from moving ahead? If the timing has changed, just tell me and I will follow up later.\n\n{{signature}}');
