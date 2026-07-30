BEGIN;

CREATE TABLE IF NOT EXISTS organizations (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY,
    org_id      UUID NOT NULL REFERENCES organizations(id),
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL UNIQUE,
    key_prefix  TEXT,
    scopes      TEXT[] NOT NULL DEFAULT '{}',
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO organizations (id, name)
VALUES ('00000000-0000-4000-8000-000000000001', 'QueueFlow Development')
ON CONFLICT (id) DO NOTHING;

-- Development-only credential: queueflow-dev-key
INSERT INTO api_keys (id, org_id, name, key_hash, key_prefix, scopes)
VALUES (
    '00000000-0000-4000-8000-000000000002',
    '00000000-0000-4000-8000-000000000001',
    'Local development',
    '8f92ffcac1e89bc504327e04105905207bed29adfe9b49b741ddfe0591872307',
    'queueflo',
    ARRAY['tasks:read', 'tasks:write', 'workers:read', 'workers:write']
)
ON CONFLICT (key_hash) DO NOTHING;

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS org_id UUID,
    ADD COLUMN IF NOT EXISTS request_fingerprint TEXT,
    ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS backoff_strategy TEXT NOT NULL DEFAULT 'exponential',
    ADD COLUMN IF NOT EXISTS task_timeout_ms BIGINT NOT NULL DEFAULT 60000,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS trace_id TEXT,
    ADD COLUMN IF NOT EXISTS dispatched_at TIMESTAMPTZ;

UPDATE tasks
SET org_id = '00000000-0000-4000-8000-000000000001'
WHERE org_id IS NULL;

UPDATE tasks
SET request_fingerprint = 'legacy:' || id::text
WHERE request_fingerprint IS NULL;

ALTER TABLE tasks
    ALTER COLUMN org_id SET NOT NULL,
    ALTER COLUMN request_fingerprint SET NOT NULL;

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_org_id_fkey,
    ADD CONSTRAINT tasks_org_id_fkey
        FOREIGN KEY (org_id) REFERENCES organizations(id);

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS chk_status;
ALTER TABLE tasks ADD CONSTRAINT chk_status
    CHECK (status IN ('pending','processing','completed','failed','dead_letter','cancelled'));

DROP INDEX IF EXISTS uq_tasks_queue_idempotency;
CREATE UNIQUE INDEX IF NOT EXISTS uq_tasks_org_queue_idempotency
    ON tasks (org_id, queue, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_org_queue_status_created
    ON tasks (org_id, queue, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_org_status_created
    ON tasks (org_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_status_scheduled
    ON tasks (status, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_tasks_org_worker
    ON tasks (org_id, locked_by);
CREATE INDEX IF NOT EXISTS idx_tasks_deleted
    ON tasks (deleted_at);
CREATE INDEX IF NOT EXISTS idx_tasks_undispatched
    ON tasks (org_id, queue, created_at)
    WHERE status = 'pending' AND dispatched_at IS NULL;

CREATE TABLE IF NOT EXISTS workers (
    org_id              UUID NOT NULL REFERENCES organizations(id),
    worker_id           TEXT NOT NULL,
    queue               TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'online',
    last_heartbeat_at   TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, worker_id)
);

CREATE INDEX IF NOT EXISTS idx_workers_org_worker
    ON workers (org_id, worker_id);

COMMIT;
