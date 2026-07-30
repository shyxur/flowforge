CREATE TABLE metric_events (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    source VARCHAR(32) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    queue VARCHAR(64),
    status VARCHAR(64),
    duration_ms BIGINT CHECK (duration_ms IS NULL OR duration_ms >= 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object' AND octet_length(metadata::text) <= 2048),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_metric_events_org_time
    ON metric_events (org_id, occurred_at DESC, id DESC);
CREATE INDEX idx_metric_events_org_source_event_time
    ON metric_events (org_id, source, event_type, occurred_at DESC, id DESC);
CREATE INDEX idx_metric_events_org_resource_time
    ON metric_events (org_id, resource_type, resource_id, occurred_at DESC, id DESC);

CREATE FUNCTION reject_metric_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'metric_events is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER metric_events_append_only
    BEFORE UPDATE OR DELETE ON metric_events
    FOR EACH ROW EXECUTE FUNCTION reject_metric_event_mutation();
