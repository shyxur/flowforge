# TaskCanvas workflows

TaskCanvas stores organization-scoped workflow drafts, validates their graphs,
publishes immutable sequential version snapshots, and executes published
versions through durable node-level orchestration. It does not provide a visual
execution timeline UI. The dashboard includes a visual workflow editor MVP for
draft creation, graph configuration, validation, publishing, and immutable
version inspection.

## Dashboard workflow editor

The dashboard exposes three TaskCanvas routes:

```text
/workflows       workflow list
/workflows/new   draft creation
/workflows/{id}  visual editor
```

The list is tenant-scoped, ordered by most recently updated, cursor-paginated,
and shows draft/published status plus the latest published version. Creating a
workflow requires a name and starts a safe draft with one trigger node.

The editor has a toolbar, visual canvas, and configuration panel. Its node
palette supports trigger, task, webhook, delay, and condition nodes. Nodes can
be moved, selected, configured, connected, and deleted. Task payloads and
webhook payloads accept JSON without adding a code-editor dependency. Webhook
nodes select only active EventForge endpoints and never expose endpoint
secrets. Delay values are persisted as `duration_seconds` and limited to seven
days. Conditions support only `input.*` fields with `equals`, `not_equals`, or
`exists`.

Connections reject self-loops, duplicate source/target pairs, and incoming
edges to triggers before reaching the API. Condition nodes expose explicit
`true` and `false` handles; their edges persist the selected branch as
`{"branch": true}` or `{"branch": false}`. Backend graph validation remains
authoritative.

Save is explicit; there is no autosave or undo/redo. Node positions are stored
as optional, backward-compatible metadata inside the definition:

```json
{
  "id": "send-email",
  "type": "task",
  "name": "Send email",
  "position": {"x": 400, "y": 160},
  "config": {"queue": "email"}
}
```

Legacy definitions without positions receive deterministic fallback positions
in the editor. Saving a published workflow follows the existing API semantics:
it creates a new draft while prior published versions remain immutable.
Archived workflows are read-only.

Validate saves local changes first and displays structured API errors. Publish
saves, validates, asks for confirmation, and creates the next immutable
version only when valid. The versions dialog lists newest first and presents a
read-only JSON snapshot. Rollback, templates, collaboration, arbitrary
expressions, execution timeline UI, and live execution visualization are not
implemented.

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
POST   /v1/workflows/{id}/executions
GET    /v1/workflows/{id}/executions?status=running&limit=50&cursor=...
GET    /v1/workflows/{id}/executions/{execution_id}
POST   /v1/workflows/{id}/executions/{execution_id}/cancel
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
`position` is the only optional canvas presentation field and does not affect
graph validation or execution.

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

Condition nodes may have multiple outgoing branches. Validation error objects
have stable `code`, `message`, and, where applicable, `path` fields.

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

## Execution architecture

Every execution references one immutable `workflow_versions` row. Postgres is
the source of truth for the execution and each node. The producer runs a small
reconciliation loop, controlled by `WORKFLOW_RECONCILE_INTERVAL_SEC` (default
`1`), which claims runnable nodes with compare-and-set updates, observes
external QueueFlow tasks and EventForge deliveries, advances dependencies, and
finalizes the execution.

Transactions cover execution creation, initial node rows, cancellation, and
terminal finalization. Redis enqueue and EventForge delivery creation happen
outside database transactions. Deterministic dispatch identities make recovery
safe when the producer restarts between an external dispatch and a state
update.

Execution statuses are `pending`, `running`, `succeeded`, `failed`, and
`cancelled`. Node statuses are `pending`, `queued`, `running`, `succeeded`,
`failed`, `skipped`, and `cancelled`.

## Starting and inspecting executions

Starting an execution requires `Idempotency-Key`. The key is unique within the
organization. Repeating the same key and canonical request returns the original
execution; reusing it for a different workflow, version, or input returns
`409 idempotency_conflict`.

The workflow itself must currently be `published`. Omitting `version` selects
the latest published snapshot. An explicit version must belong to the same
workflow and organization. Later publishes never change the snapshot selected
by an existing execution.

```bash
curl -i http://localhost:8080/v1/workflows/WORKFLOW_ID/executions \
  -X POST \
  -H 'Authorization: Bearer queueflow-dev-key' \
  -H 'Idempotency-Key: order-cus-123-created' \
  -H 'Content-Type: application/json' \
  -d '{
    "version": 2,
    "input": {
      "customer_id": "cus_123",
      "status": "paid"
    }
  }'
```

The asynchronous start returns `202`:

```json
{
  "execution_id": "69ec21e1-32d9-4d08-83cc-e0abfddc6494",
  "workflow_id": "8d55f741-a2f6-4461-b21b-f4f7795ed172",
  "workflow_version": 2,
  "status": "running",
  "created_at": "2026-07-30T12:45:00Z"
}
```

Execution lists use the existing opaque cursor scheme and return newest first.
Execution detail includes input, output, structured `error_code` and
`error_message` fields, and node states ordered by creation time and node ID:

```json
{
  "execution_id": "69ec21e1-32d9-4d08-83cc-e0abfddc6494",
  "workflow_id": "8d55f741-a2f6-4461-b21b-f4f7795ed172",
  "workflow_version_id": "1130f00d-2b1b-4c51-884f-3a83545d7506",
  "workflow_version": 2,
  "status": "running",
  "input": {"customer_id": "cus_123", "status": "paid"},
  "nodes": [
    {
      "node_id": "order-created",
      "node_type": "trigger",
      "status": "succeeded",
      "attempt": 1
    },
    {
      "node_id": "charge-card",
      "node_type": "task",
      "status": "queued",
      "attempt": 1,
      "queue_task_id": "d8e38839-ff5a-4ace-8509-d34c9194317e"
    }
  ]
}
```

## Executable node configurations

Trigger nodes are all entry points. They succeed immediately, receive the
execution input, and perform no external work.

Task nodes enqueue into QueueFlow. `queue` is required. `payload` is optional
and defaults to the execution input. `priority`, `max_retries`, and
`timeout_seconds` are optional and use QueueFlow bounds.

```json
{
  "queue": "payments",
  "payload": {"kind": "charge"},
  "priority": 5,
  "max_retries": 3,
  "timeout_seconds": 60
}
```

Webhook nodes require an existing active EventForge `endpoint_id`. Their
optional payload defaults to the execution input. EventForge performs signing,
delivery retries, and terminal failure handling with the existing
`X-Windylane-*` headers. The internal event name is `workflow.node`.

```json
{
  "endpoint_id": "00000000-0000-4000-8000-000000000002",
  "payload": {"kind": "workflow.notification"}
}
```

Delay nodes use a durable due time and never sleep in a worker goroutine.
`duration_seconds` must be between `0` and `604800` (seven days).

```json
{"duration_seconds": 30}
```

Condition nodes evaluate only execution input. Operators are `equals`,
`not_equals`, and `exists`; fields must start with `input.`. No arbitrary
expressions or user code are supported.

```json
{"field": "input.status", "operator": "equals", "value": "paid"}
```

Outgoing condition edges may be unlabeled, or may select a boolean branch:

```json
{"id": "paid", "from": "condition", "to": "charge", "condition": {"branch": true}}
```

Unselected nodes become `skipped` only after every incoming dependency is
resolved and none remains active. A node with multiple active incoming edges
waits for all of them to succeed.

## Retry, recovery, and cancellation

QueueFlow owns task retry, backoff, visibility timeouts, and DLQ behavior.
EventForge owns webhook retry and signing. The workflow node mirrors the
external attempt count and becomes terminal only when the underlying work
succeeds or fails terminally. Conditions and trigger evaluation are immediate;
delays are resumed by durable reconciliation and do not have a separate retry
system.

Reconciliation scans `pending` and `running` executions after startup and
periodically thereafter. Node claims are conditional, terminal updates are
idempotent, task idempotency keys derive from execution and node IDs, and
webhook delivery IDs are deterministic. Duplicate reconciliation or completion
observations therefore do not dispatch a node twice.
An unlinked `running` claim is treated as in-flight for 30 seconds before
reconciliation retries its deterministic dispatch, avoiding races with the
original dispatcher while still recovering after a process crash.

Pending or running executions may be cancelled. Cancellation atomically marks
the execution and every open node `cancelled`; terminal executions return
`409 workflow_execution_terminal`. QueueFlow tasks or EventForge deliveries
already in flight may still finish externally, but cancellation prevents any
new downstream workflow node from starting.

## Current boundaries

TaskCanvas does not yet implement rollback, arbitrary condition languages,
distributed DAG scheduling beyond this durable reconciler, execution timeline
UI, live execution paths, advanced templates, autosave, or collaborative
editing.
