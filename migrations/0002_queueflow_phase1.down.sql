BEGIN;

DROP TABLE IF EXISTS workers;
DROP INDEX IF EXISTS idx_tasks_undispatched;
DROP INDEX IF EXISTS idx_tasks_deleted;
DROP INDEX IF EXISTS idx_tasks_org_worker;
DROP INDEX IF EXISTS idx_tasks_status_scheduled;
DROP INDEX IF EXISTS idx_tasks_org_status_created;
DROP INDEX IF EXISTS idx_tasks_org_queue_status_created;
DROP INDEX IF EXISTS uq_tasks_org_queue_idempotency;

CREATE UNIQUE INDEX IF NOT EXISTS uq_tasks_queue_idempotency
    ON tasks (queue, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS chk_status;
ALTER TABLE tasks ADD CONSTRAINT chk_status
    CHECK (status IN ('pending','processing','completed','failed','dead_letter'));

ALTER TABLE tasks
    DROP COLUMN IF EXISTS dispatched_at,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS task_timeout_ms,
    DROP COLUMN IF EXISTS backoff_strategy,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS request_fingerprint,
    DROP COLUMN IF EXISTS org_id;

DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS organizations;

COMMIT;
