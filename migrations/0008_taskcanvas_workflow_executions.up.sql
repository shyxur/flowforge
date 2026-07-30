BEGIN;

ALTER TABLE workflow_versions
    ADD CONSTRAINT workflow_versions_org_workflow_id_version_unique
    UNIQUE (org_id, workflow_id, id, version);

CREATE TABLE workflow_executions (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    workflow_id UUID NOT NULL,
    workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id),
    workflow_version INTEGER NOT NULL CHECK (workflow_version > 0),
    status TEXT NOT NULL,
    input JSONB,
    output JSONB,
    error_code TEXT,
    error_message TEXT,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT workflow_executions_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT workflow_executions_org_workflow_fk
        FOREIGN KEY (org_id, workflow_id) REFERENCES workflows(org_id, id),
    CONSTRAINT workflow_executions_version_fk
        FOREIGN KEY (org_id, workflow_id, workflow_version_id, workflow_version)
        REFERENCES workflow_versions(org_id, workflow_id, id, version),
    CONSTRAINT workflow_executions_org_id_unique UNIQUE (org_id, id),
    CONSTRAINT workflow_executions_org_idempotency_unique UNIQUE (org_id, idempotency_key)
);

CREATE TABLE workflow_node_executions (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    workflow_execution_id UUID NOT NULL,
    node_id TEXT NOT NULL,
    node_type TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    input JSONB,
    output JSONB,
    error_code TEXT,
    error_message TEXT,
    queue_task_id UUID REFERENCES tasks(id),
    webhook_delivery_id UUID REFERENCES webhook_deliveries(id),
    available_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT workflow_node_executions_type_check
        CHECK (node_type IN ('trigger', 'task', 'webhook', 'condition', 'delay')),
    CONSTRAINT workflow_node_executions_status_check
        CHECK (status IN ('pending', 'queued', 'running', 'succeeded', 'failed', 'skipped', 'cancelled')),
    CONSTRAINT workflow_node_executions_execution_fk
        FOREIGN KEY (org_id, workflow_execution_id)
        REFERENCES workflow_executions(org_id, id),
    CONSTRAINT workflow_node_executions_org_execution_node_unique
        UNIQUE (org_id, workflow_execution_id, node_id)
);

CREATE INDEX idx_workflow_executions_org_workflow_created
    ON workflow_executions (org_id, workflow_id, created_at DESC, id DESC);
CREATE INDEX idx_workflow_executions_org_status_created
    ON workflow_executions (org_id, status, created_at DESC, id DESC);
CREATE INDEX idx_workflow_node_executions_org_execution
    ON workflow_node_executions (org_id, workflow_execution_id, created_at, node_id);
CREATE INDEX idx_workflow_executions_reconcile
    ON workflow_executions (updated_at, id)
    WHERE status IN ('pending', 'running');

ALTER TABLE webhook_deliveries
    DROP CONSTRAINT chk_webhook_delivery_event_type;
ALTER TABLE webhook_deliveries
    ADD CONSTRAINT chk_webhook_delivery_event_type CHECK (
        event_type IN (
            'task.created',
            'task.processing',
            'task.completed',
            'task.failed',
            'task.dead_letter',
            'task.cancelled',
            'workflow.node'
        )
    );

COMMIT;
