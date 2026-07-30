BEGIN;

CREATE TABLE workflows (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'draft',
    definition JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT workflows_status_check CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT workflows_definition_object_check CHECK (jsonb_typeof(definition) = 'object')
);

CREATE INDEX idx_workflows_org ON workflows (org_id);
CREATE INDEX idx_workflows_org_slug ON workflows (org_id, slug);
CREATE INDEX idx_workflows_org_status ON workflows (org_id, status);
CREATE INDEX idx_workflows_org_created ON workflows (org_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX uq_workflows_org_slug_active
    ON workflows (org_id, slug)
    WHERE deleted_at IS NULL;

COMMIT;
