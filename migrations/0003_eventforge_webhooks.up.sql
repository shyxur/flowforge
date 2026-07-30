BEGIN;

CREATE TABLE webhook_endpoints (
    id          UUID PRIMARY KEY,
    org_id      UUID NOT NULL REFERENCES organizations(id),
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    event_types TEXT[] NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_webhook_endpoint_event_types CHECK (
        cardinality(event_types) > 0
        AND event_types <@ ARRAY[
            'task.created',
            'task.processing',
            'task.completed',
            'task.failed',
            'task.dead_letter',
            'task.cancelled'
        ]::TEXT[]
    ),
    UNIQUE (org_id, id)
);

CREATE TABLE webhook_deliveries (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES organizations(id),
    endpoint_id     UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL,
    attempt_count   INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 5,
    next_attempt_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    response_status INT,
    response_body   TEXT,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_webhook_delivery_endpoint
        FOREIGN KEY (org_id, endpoint_id) REFERENCES webhook_endpoints(org_id, id),
    CONSTRAINT chk_webhook_delivery_event_type CHECK (
        event_type IN (
            'task.created',
            'task.processing',
            'task.completed',
            'task.failed',
            'task.dead_letter',
            'task.cancelled'
        )
    ),
    CONSTRAINT chk_webhook_delivery_status CHECK (
        status IN ('pending', 'processing', 'succeeded', 'failed', 'dead_letter')
    ),
    CONSTRAINT chk_webhook_delivery_attempts CHECK (
        attempt_count >= 0 AND max_attempts > 0 AND attempt_count <= max_attempts
    )
);

CREATE INDEX idx_webhook_endpoints_org_active_created
    ON webhook_endpoints (org_id, is_active, created_at DESC);
CREATE INDEX idx_webhook_endpoints_org_deleted
    ON webhook_endpoints (org_id, deleted_at);
CREATE INDEX idx_webhook_deliveries_org_endpoint_created
    ON webhook_deliveries (org_id, endpoint_id, created_at DESC);
CREATE INDEX idx_webhook_deliveries_status_next_attempt
    ON webhook_deliveries (status, next_attempt_at);
CREATE INDEX idx_webhook_deliveries_org_status_created
    ON webhook_deliveries (org_id, status, created_at DESC);

COMMIT;
