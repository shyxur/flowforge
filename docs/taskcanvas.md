# TaskCanvas workflow validation and publishing

TaskCanvas stores organization-scoped workflow drafts, validates their graphs,
and publishes immutable sequential version snapshots. It does not yet execute
workflows or provide a visual editor.

## API

All routes require the existing Bearer API key and are scoped to its
organization:

```text
POST   /v1/workflows
GET    /v1/workflows?status=draft&limit=50&cursor=...
GET    /v1/workflows/{id}
PATCH  /v1/workflows/{id}
DELETE /v1/workflows/{id}
POST   /v1/workflows/{id}/validate
POST   /v1/workflows/{id}/publish
GET    /v1/workflows/{id}/versions
GET    /v1/workflows/{id}/versions/{version}
```

`POST /v1/workflows` creates a draft. `PATCH` accepts `name`, `slug`,
`description`, and `definition`. Editing a published workflow starts a new
draft by changing its status back to `draft`; existing published snapshots
remain unchanged. Archived workflows cannot be changed or published. `DELETE`
soft-deletes the workflow and hides it and its versions from normal reads.

## Definition format

Both `nodes` and `edges` arrays are required. Supported node types are
`trigger`, `task`, `webhook`, `condition`, and `delay`.

```json
{
  "nodes": [
    {
      "id": "order-created",
      "type": "trigger",
      "name": "Order created",
      "config": {}
    },
    {
      "id": "charge-card",
      "type": "task",
      "name": "Charge card",
      "config": {
        "queue": "payments"
      }
    },
    {
      "id": "notify",
      "type": "webhook",
      "name": "Notify storefront",
      "config": {
        "endpoint_id": "00000000-0000-4000-8000-000000000002"
      }
    }
  ],
  "edges": [
    {
      "id": "order-to-charge",
      "from": "order-created",
      "to": "charge-card",
      "condition": null
    },
    {
      "id": "charge-to-notify",
      "from": "charge-card",
      "to": "notify",
      "condition": null
    }
  ]
}
```

Draft ingestion enforces the JSON shape, supported node types, non-empty and
unique node/edge IDs, object-shaped node configs, and valid edge references.
Unknown fields and malformed definitions return `400 validation_failed`.

## Publish validation

`POST /v1/workflows/{id}/validate` runs the same graph checks used by publish.
Graph problems are a successful validation operation and therefore return
HTTP `200`:

```json
{
  "valid": false,
  "errors": [
    {
      "code": "unreachable_node",
      "message": "non-trigger nodes must be reachable from a trigger",
      "path": "nodes[2]"
    }
  ]
}
```

A publishable graph must:

- contain at least one trigger and at least one task, webhook, delay, or
  condition node;
- keep trigger nodes free of incoming edges;
- make every non-trigger node reachable from a trigger;
- contain no isolated/orphan nodes;
- contain no cycles, self-loops, or duplicate source-to-target edges;
- use unique IDs and reference only nodes in the same definition;
- use object-shaped node configs and object-or-null edge conditions.

Condition nodes may have multiple outgoing branches. TaskCanvas does not yet
enforce condition branch semantics or deep execution-specific config schemas.
Validation error objects have stable `code`, `message`, and, where applicable,
`path` fields.

## Publishing and versions

`POST /v1/workflows/{id}/publish` validates and publishes atomically. The
transaction locks the current workflow, assigns `MAX(version) + 1`, inserts a
snapshot in `workflow_versions`, and marks the workflow `published`.

```json
{
  "workflow_id": "8d55f741-a2f6-4461-b21b-f4f7795ed172",
  "version": 1,
  "version_id": "88e47659-55f9-43c0-a813-6fb518907b1c",
  "status": "published",
  "published_at": "2026-07-30T12:00:00Z"
}
```

The first publish is version 1; every later publish creates the next version,
including when the definition is unchanged. Version rows have no update API.
Their name, slug, description, and definition are immutable snapshots.

The versions list returns newest first:

```json
{
  "items": [
    {
      "version": 2,
      "version_id": "1130f00d-2b1b-4c51-884f-3a83545d7506",
      "status": "published",
      "published_at": "2026-07-30T12:30:00Z",
      "name": "Order lifecycle",
      "slug": "order-lifecycle"
    }
  ]
}
```

Publishing an invalid graph returns `400 workflow_validation_failed` with the
structured errors in `error.details.errors`. Missing, deleted, or cross-org
workflows return the same tenant-safe `404` behavior as the draft CRUD API.

## Current boundaries

TaskCanvas does not yet implement rollback, workflow execution, execution APIs,
execution timelines, or dashboard canvas/editor pages.
