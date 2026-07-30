BEGIN;

ALTER TABLE workflows
    ADD CONSTRAINT workflows_org_id_id_unique UNIQUE (org_id, id);

CREATE TABLE workflow_versions (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    workflow_id UUID NOT NULL REFERENCES workflows(id),
    version INTEGER NOT NULL CHECK (version > 0),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    definition JSONB NOT NULL,
    status TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT workflow_versions_status_check CHECK (status IN ('published', 'deprecated')),
    CONSTRAINT workflow_versions_definition_object_check CHECK (jsonb_typeof(definition) = 'object'),
    CONSTRAINT workflow_versions_org_workflow_version_unique UNIQUE (org_id, workflow_id, version),
    CONSTRAINT workflow_versions_org_workflow_fk
        FOREIGN KEY (org_id, workflow_id) REFERENCES workflows(org_id, id)
);

CREATE INDEX idx_workflow_versions_org_workflow
    ON workflow_versions (org_id, workflow_id);
CREATE INDEX idx_workflow_versions_org_workflow_published
    ON workflow_versions (org_id, workflow_id, version DESC);
CREATE INDEX idx_workflow_versions_org_slug
    ON workflow_versions (org_id, slug);

COMMIT;
