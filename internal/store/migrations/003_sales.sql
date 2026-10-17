-- Sales: what was sold, to whom, for how much and for how long. Monthly and yearly plans make recurring revenue
-- (MRR and ARR come from the active ones); one-time sales are licences or set-up work.
CREATE TABLE sales (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id        UUID        REFERENCES leads (id) ON DELETE SET NULL,
    customer       TEXT        NOT NULL,
    company        TEXT        NOT NULL DEFAULT '',
    email          TEXT        NOT NULL DEFAULT '',
    plan           TEXT        NOT NULL DEFAULT '',
    billing        TEXT        NOT NULL,
    amount         BIGINT      NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'active',
    started_on     DATE        NOT NULL DEFAULT current_date,
    ended_on       DATE,
    cancel_reason  TEXT        NOT NULL DEFAULT '',
    owner_id       UUID        REFERENCES users (id) ON DELETE SET NULL,
    notes          TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT sales_billing_check CHECK (billing IN ('monthly', 'yearly', 'one_time')),
    CONSTRAINT sales_status_check CHECK (status IN ('active', 'cancelled')),
    CONSTRAINT sales_amount_check CHECK (amount >= 0)
);
CREATE INDEX sales_status_idx ON sales (status, started_on DESC);
CREATE INDEX sales_lead_idx ON sales (lead_id);
CREATE INDEX sales_owner_idx ON sales (owner_id);
