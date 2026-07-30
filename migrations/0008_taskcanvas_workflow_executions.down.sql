BEGIN;

DROP TABLE IF EXISTS workflow_node_executions;
DROP TABLE IF EXISTS workflow_executions;

DELETE FROM webhook_deliveries WHERE event_type = 'workflow.node';

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
            'task.cancelled'
        )
    );

ALTER TABLE workflow_versions
    DROP CONSTRAINT IF EXISTS workflow_versions_org_workflow_id_version_unique;

COMMIT;
