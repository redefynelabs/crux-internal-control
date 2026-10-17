-- Licensing, activation, releases and an audit trail.

-- Lifecycle and what the installation last told us. status is the vendor's decision (suspend, revoke); every other state
-- (awaiting activation, active, out of date, unreachable, expired) is worked out from these columns and the newest file.
ALTER TABLE installations
    ADD COLUMN status             TEXT        NOT NULL DEFAULT 'active',
    ADD COLUMN status_note        TEXT        NOT NULL DEFAULT '',
    ADD COLUMN status_changed_at  TIMESTAMPTZ,
    ADD COLUMN activated_at       TIMESTAMPTZ,
    ADD COLUMN last_seen_at       TIMESTAMPTZ,
    ADD COLUMN applied_version    INT,
    ADD COLUMN server_version     TEXT        NOT NULL DEFAULT '',
    ADD COLUMN server_build       TEXT        NOT NULL DEFAULT '',
    ADD CONSTRAINT installations_status_check CHECK (status IN ('active', 'suspended', 'revoked'));

-- Demos, and installations that were wired up by hand before activation existed, count as activated.
UPDATE installations SET activated_at = created_at;

-- One-time codes a customer types at setup. Only a hash is kept; the code is shown once.
CREATE TABLE activation_codes (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    installation_id  UUID        NOT NULL REFERENCES installations (id) ON DELETE CASCADE,
    code_hash        TEXT        NOT NULL UNIQUE,
    hint             TEXT        NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    used_at          TIMESTAMPTZ,
    revoked_at       TIMESTAMPTZ,
    created_by       UUID        REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX activation_codes_installation_idx ON activation_codes (installation_id, created_at DESC);

-- Releases of the ERP. The manifest is the signed document installations verify; the columns are for the back office.
CREATE TABLE releases (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    version           TEXT        NOT NULL UNIQUE,
    channel           TEXT        NOT NULL DEFAULT 'stable',
    published_at      TIMESTAMPTZ NOT NULL,
    notes             TEXT        NOT NULL DEFAULT '',
    has_migrations    BOOLEAN     NOT NULL DEFAULT false,
    min_from_version  TEXT        NOT NULL DEFAULT '',
    images            JSONB       NOT NULL DEFAULT '{}',
    manifest          TEXT        NOT NULL,
    withdrawn_at      TIMESTAMPTZ,
    created_by        UUID        REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT releases_channel_check CHECK (channel IN ('stable', 'beta', 'security'))
);
CREATE INDEX releases_published_idx ON releases (published_at DESC);

-- Who did what to an installation or a release.
CREATE TABLE audit_log (
    id               BIGSERIAL   PRIMARY KEY,
    at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id         UUID        REFERENCES users (id) ON DELETE SET NULL,
    actor_email      TEXT        NOT NULL DEFAULT '',
    installation_id  UUID        REFERENCES installations (id) ON DELETE CASCADE,
    action           TEXT        NOT NULL,
    detail           JSONB       NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_log_installation_idx ON audit_log (installation_id, at DESC);
CREATE INDEX audit_log_at_idx ON audit_log (at DESC);
