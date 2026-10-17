-- Installations of the ERP the vendor issues entitlements to, the signed files issued to them, and what they asked for.
CREATE TABLE installations (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT        NOT NULL,
    kind               TEXT        NOT NULL DEFAULT 'customer',
    demo_id            UUID        REFERENCES demos (id) ON DELETE SET NULL,
    lead_id            UUID        REFERENCES leads (id) ON DELETE SET NULL,
    -- The credential an installation signs its calls with is derived from this number; raising it replaces the credential.
    secret_generation  INT         NOT NULL DEFAULT 1,
    notes              TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT installations_kind_check CHECK (kind IN ('customer', 'demo'))
);
CREATE INDEX installations_kind_idx ON installations (kind, created_at DESC);

-- Every file issued, kept for good: the version only goes up, and the newest one is what the installation is given.
CREATE TABLE entitlements (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    installation_id  UUID        NOT NULL REFERENCES installations (id) ON DELETE CASCADE,
    version          INT         NOT NULL,
    payload          JSONB       NOT NULL,
    file             TEXT        NOT NULL,
    issued_by        UUID        REFERENCES users (id) ON DELETE SET NULL,
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (installation_id, version)
);

-- Asks from installations. The id is the one the installation gave, so a retried post changes nothing.
CREATE TABLE access_requests (
    id               UUID        NOT NULL,
    installation_id  UUID        NOT NULL REFERENCES installations (id) ON DELETE CASCADE,
    kind             TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    note             TEXT        NOT NULL DEFAULT '',
    requested_at     TIMESTAMPTZ NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'pending',
    decided_note     TEXT        NOT NULL DEFAULT '',
    decided_at       TIMESTAMPTZ,
    decided_by       UUID        REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (installation_id, id),
    CONSTRAINT access_requests_kind_check CHECK (kind IN ('app', 'feature')),
    CONSTRAINT access_requests_status_check CHECK (status IN ('pending', 'approved', 'declined', 'withdrawn'))
);
CREATE INDEX access_requests_status_idx ON access_requests (status, created_at DESC);
