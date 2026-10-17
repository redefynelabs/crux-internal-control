CREATE TABLE leads (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    email       TEXT        NOT NULL,
    phone       TEXT        NOT NULL DEFAULT '',
    company     TEXT        NOT NULL DEFAULT '',
    source      TEXT        NOT NULL DEFAULT 'website',
    status      TEXT        NOT NULL DEFAULT 'new',
    notes       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT leads_status_check CHECK (status IN ('new', 'demo_active', 'demo_ended', 'contacted', 'qualified', 'won', 'lost'))
);
CREATE UNIQUE INDEX leads_email_key ON leads (lower(email));
CREATE INDEX leads_status_idx ON leads (status, created_at DESC);

-- What happened to a lead, newest last: demo started, demo ended, status changed, note added.
CREATE TABLE lead_events (
    id       BIGSERIAL   PRIMARY KEY,
    lead_id  UUID        NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    kind     TEXT        NOT NULL,
    detail   JSONB       NOT NULL DEFAULT '{}',
    at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX lead_events_lead_idx ON lead_events (lead_id, at);

-- One row per demo instance. The slug names its database, containers and address.
CREATE TABLE demos (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id     UUID        NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
    slug        TEXT        NOT NULL UNIQUE,
    status      TEXT        NOT NULL DEFAULT 'provisioning',
    url         TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT '',
    error       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ,
    CONSTRAINT demos_status_check CHECK (status IN ('provisioning', 'active', 'failed', 'expired'))
);
CREATE INDEX demos_status_idx ON demos (status, expires_at);
CREATE INDEX demos_lead_idx ON demos (lead_id, created_at DESC);
CREATE INDEX demos_ip_idx ON demos (ip, created_at DESC);
