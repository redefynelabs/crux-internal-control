-- A demo wanted while every slot is busy waits in line instead of being turned away. A queued row has its slug reserved
-- but no database or containers; expires_at is then the time the request gives up, and becomes the demo's end when it
-- starts. request keeps the visitor's choices (apps, team size, focus, attribution) until then.
ALTER TABLE demos
    ADD COLUMN queued_at TIMESTAMPTZ,
    ADD COLUMN request   JSONB NOT NULL DEFAULT '{}',
    DROP CONSTRAINT demos_status_check,
    ADD CONSTRAINT demos_status_check CHECK (status IN ('queued', 'provisioning', 'active', 'failed', 'expired', 'abandoned'));
CREATE INDEX demos_queue_idx ON demos (queued_at) WHERE status = 'queued';
