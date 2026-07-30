BEGIN;

DROP TABLE IF EXISTS workflow_versions;

ALTER TABLE workflows
    DROP CONSTRAINT IF EXISTS workflows_org_id_id_unique;

COMMIT;
