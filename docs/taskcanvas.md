# TaskCanvas workflow foundations

TaskCanvas M1 adds authenticated, organization-scoped storage and CRUD for
draft workflow definitions. It is a backend foundation for Phase 3, not a
workflow runner or visual editor.

## API

All routes require the existing Bearer API key:

```text
POST   /v1/workflows
GET    /v1/workflows?status=draft&limit=50&cursor=...
GET    /v1/workflows/{id}
PATCH  /v1/workflows/{id}
DELETE /v1/workflows/{id}
```

`POST` always creates a `draft`. `PATCH` accepts only `name`, `slug`,
`description`, and `definition`, and only draft workflows are editable.
`DELETE` soft-deletes a workflow. Normal list and detail reads never expose
deleted records or records from another organization.

List results use the existing opaque cursor format:

```json
{
  "items": [],
  "next_cursor": "opaque-value"
}
```

## Definition format

```json
{
  "nodes": [
    {
      "id": "start",
      "type": "trigger",
      "name": "Start",
      "config": {}
    }
  ],
  "edges": []
}
```

Both arrays are required, including when empty. Node and edge IDs must be
present and unique. Every edge `from` and `to` value must reference a node in
the same definition. Supported node types are `trigger`, `task`, `webhook`,
`condition`, and `delay`. Unknown fields and invalid JSON shapes return the
standard `400 validation_failed` response.

Workflow names are trimmed and limited to 255 bytes. Descriptions are optional
and limited to 2,000 bytes. Slugs may be supplied or generated from the name;
they must contain lowercase ASCII letters, digits, and single hyphen separators,
with a maximum length of 120 bytes. Active slugs are unique within an
organization. A duplicate returns `409 workflow_slug_conflict`.

## M1 boundaries

M1 does not include cycle detection, publish validation, versioning, workflow
execution, a visual canvas, or dashboard workflow pages. Those remain later
TaskCanvas milestones.
