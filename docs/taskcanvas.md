# TaskCanvas workflows

TaskCanvas stores organization-scoped workflow drafts, validates their graphs,
publishes immutable sequential version snapshots, and executes published
versions through durable node-level orchestration. The dashboard includes a
visual workflow editor for draft creation, graph configuration, validation,
publishing, immutable version inspection, and execution timeline monitoring.

TaskCanvas is windylane's workflow layer:

- QueueFlow executes task nodes with its existing durable queue, retry,
  backoff, timeout, visibility, and DLQ behavior.
- EventForge executes webhook nodes with its existing endpoint isolation,
  signing, delivery retry, and `X-Windylane-*` protocol.
- TaskCanvas owns workflow drafts, graph validation, immutable versions,
  execution/node state, dependency advancement, durable delays, deterministic
  conditions, reconciliation, and cancellation.

Postgres remains the durable source of truth. Redis is used by QueueFlow for
task transport; it is not the workflow execution database. All TaskCanvas API
operations are authenticated, organization-scoped, and tenant-safe.

## Local development values

```text
API              http://localhost:8080
Dashboard        http://localhost:3000
Organization ID  00000000-0000-4000-8000-000000000001
API key          queueflow-dev-key
```

The organization ID identifies the seeded local tenant, but clients do not
send it as a separate header. The API resolves the organization from:

```http
Authorization: Bearer queueflow-dev-key
```

## Workflow lifecycle

1. Create an organization-scoped draft.
2. Edit its metadata, nodes, edges, positions, and node configuration.
3. Validate the graph. Validation returns structured errors without publishing.
4. Publish atomically. Version 1, then version 2 and later versions, are
   immutable snapshots of the workflow metadata and definition.
5. Start an execution against the latest published version, or select an
   explicit version.
6. Inspect the execution record and its durable node execution records.
7. Cancel a `pending` or `running` execution when required.
8. Edit the workflow again. Updating a published workflow creates a new draft;
   publishing it creates the next immutable version.
9. Existing executions and their timelines remain tied to the original
   version, even after later edits and publishes.

Soft deletion hides a workflow from normal reads without rewriting published
versions or historical execution records. Archived workflows are read-only.

## Dashboard guide

The dashboard exposes five TaskCanvas routes:

```text
/workflows       workflow list
/workflows/new   draft creation
/workflows/{id}  visual editor
/workflows/{id}/executions
                  execution list and run dialog
/workflows/{id}/executions/{execution_id}
                  execution timeline and node detail
```

Use the dashboard as follows:

1. Open `/workflows` to filter or paginate the tenant-scoped workflow list.
2. Select **new workflow** or open `/workflows/new`. A name is required; the
   dashboard starts a safe draft with one trigger.
3. Open `/workflows/{id}`. Add trigger, task, webhook, delay, and condition
   nodes from the palette.
4. Select a node to edit its type-specific configuration. Task and webhook
   payloads accept JSON. Webhook selection includes only active endpoints.
5. Connect nodes on the canvas. Use the explicit `true` or `false` condition
   handle for conditional branches.
6. Save explicitly, then validate. Fix any structured graph/configuration
   errors shown by the editor.
7. Publish after confirmation. Open the versions dialog to inspect immutable
   snapshots newest-first.
8. Open `/workflows/{id}/executions`, choose **run workflow**, select **latest
   published** or an explicit version, and provide JSON input up to 256 KiB.
9. Filter and paginate the execution list. Open an execution to inspect its
   immutable-version timeline.
10. Cancel an active execution from its detail page when needed.

The list is ordered by most recently updated and shows draft/published status
plus the latest published version.

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
Published definitions are inspected through immutable versions; editing a
published workflow returns it to `draft` before another publish.

Validate saves local changes first and displays structured API errors. Publish
saves, validates, asks for confirmation, and creates the next immutable
version only when valid. The versions dialog lists newest first and presents a
read-only JSON snapshot. Rollback, templates, collaboration, arbitrary
expressions, autosave, and undo/redo are not implemented.

## Dashboard execution monitoring

The execution list is cursor-paginated and can be filtered by execution
status. Each row shows the immutable workflow version, status, creation and
start timestamps, duration, and a link to the detail view. Loading, empty, and
retryable error states keep the workflow context visible.

Run workflow is available only for published workflows. The dialog accepts an
optional version and JSON input. Leaving the version on **Latest published**
omits it from the request so the backend selects the current published
snapshot; selecting a version pins the run to that immutable snapshot. The
dashboard validates JSON and the 256 KiB input limit before submission and
generates a fresh idempotency key for each deliberate Run action. A successful
start redirects directly to the new execution.

The detail view loads the execution together with its exact immutable workflow
version. Nodes are rendered in deterministic topological order, with stable
node-ID ordering as a fallback for malformed or cyclic historical data. The
vertical timeline exposes each node's type, status, attempt, timing, input,
output, error, and linked QueueFlow task or EventForge delivery ID when
present. Labels and configuration come from the version snapshot, so later
workflow edits cannot rewrite historical execution context.

Pending and running detail views poll every two seconds while the page is
visible. Polling pauses in a hidden tab, resumes when visible, and stops as
soon as the execution becomes terminal. A transient refresh failure preserves
the last known timeline. Cancellation requires confirmation, refreshes the
complete detail immediately, and is available only for active executions.

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

Every endpoint requires `Authorization: Bearer <api-key>`. The authenticated
API key supplies the organization scope; a workflow, version, execution, or
cursor from another organization is never accepted. JSON requests also require
`Content-Type: application/json`. Only execution creation additionally
requires `Idempotency-Key`.

### Endpoint reference

| Endpoint | Purpose and request | Success response | Important errors |
| --- | --- | --- | --- |
| `POST /v1/workflows` | Create a draft. Body: `name` (required), `slug` (optional), `description` (optional string/null), and `definition` (required object). | `201` with the complete workflow object. | `400 invalid_json` or `validation_failed`; `409 workflow_slug_conflict`; `413 request_too_large`. |
| `GET /v1/workflows` | List tenant workflows. Optional query: `status=draft\|published\|archived`, opaque `cursor`, and `limit=1..100` (default 50). | `200 {"items":[...],"next_cursor":"..."}`. | `400 invalid_status`, `invalid_cursor`, or `invalid_limit`. |
| `GET /v1/workflows/{id}` | Read one active workflow by UUID. No body. | `200` with the complete workflow object. | `400 invalid_workflow_id`; tenant-safe `404 workflow_not_found`. |
| `PATCH /v1/workflows/{id}` | Update at least one of `name`, `slug`, `description`, or `definition`. Updating a published workflow returns it to `draft`; archived workflows are not editable. | `200` with the updated workflow object. | `400 invalid_json` or `validation_failed`; `404 workflow_not_found`; `409 workflow_slug_conflict` or `workflow_not_editable`; `413 request_too_large`. |
| `DELETE /v1/workflows/{id}` | Soft-delete a workflow. No body. | `204` with no body. | `400 invalid_workflow_id`; tenant-safe `404 workflow_not_found`. |
| `POST /v1/workflows/{id}/validate` | Validate the stored draft definition. No body. Graph failures are data, not transport failures. | `200 {"valid":true,"errors":[]}` or `200 {"valid":false,"errors":[...]}`. | `400 invalid_workflow_id`; tenant-safe `404 workflow_not_found`. |
| `POST /v1/workflows/{id}/publish` | Validate and atomically create the next immutable version. No body. | `201` with `workflow_id`, `version`, `version_id`, `status`, and `published_at`. | `400 workflow_validation_failed`; `404 workflow_not_found`; `409 workflow_publish_conflict` or `workflow_not_editable`. |
| `GET /v1/workflows/{id}/versions` | List immutable versions newest-first. No body or pagination query. | `200 {"items":[...]}` with version summaries. | `400 invalid_workflow_id`; tenant-safe `404 workflow_not_found`. |
| `GET /v1/workflows/{id}/versions/{version}` | Read one positive integer version and its full immutable definition. | `200` with `version_id`, `workflow_id`, `version`, snapshot metadata, definition, status, and timestamps. | `400 invalid_workflow_id` or `invalid_workflow_version`; tenant-safe `404 workflow_version_not_found`. |
| `POST /v1/workflows/{id}/executions` | Start the latest or an explicit published version. Header: `Idempotency-Key` (required, max 255 characters). Body: optional positive `version` and optional JSON `input`. | `202` with `execution_id`, `workflow_id`, `workflow_version`, `status`, and `created_at`. An idempotent replay returns the same result and status. | `400 missing_idempotency_key`, `invalid_idempotency_key`, or `validation_failed`; `404 workflow_version_not_found`; `409 workflow_not_published` or `idempotency_conflict`; `413 payload_too_large` or `request_too_large`. |
| `GET /v1/workflows/{id}/executions` | List executions newest-first. Optional query: execution `status`, opaque `cursor`, and `limit=1..100` (default 50). | `200 {"items":[...],"next_cursor":"..."}`. | `400 invalid_status`, `invalid_cursor`, or `invalid_limit`; tenant-safe `404` behavior. |
| `GET /v1/workflows/{id}/executions/{execution_id}` | Read an execution and its node execution records. No body. | `200` with execution fields and `nodes`. | `400` for malformed UUIDs; tenant-safe `404 workflow_execution_not_found`. |
| `POST /v1/workflows/{id}/executions/{execution_id}/cancel` | Cancel a `pending` or `running` execution. No body. | `200` with the cancelled execution record. | `400` for malformed UUIDs; `404 workflow_execution_not_found`; `409 workflow_execution_terminal`. |

API errors use one envelope:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed",
    "details": {}
  }
}
```

A workflow create/get/update item uses this shape:

```json
{
  "id": "8d55f741-a2f6-4461-b21b-f4f7795ed172",
  "org_id": "00000000-0000-4000-8000-000000000001",
  "name": "Order receipt workflow",
  "slug": "order-receipt-workflow",
  "description": "Send a receipt for paid orders or wait for payment.",
  "status": "draft",
  "definition": {
    "nodes": [],
    "edges": []
  },
  "created_at": "2026-07-30T12:00:00Z",
  "updated_at": "2026-07-30T12:00:00Z"
}
```

The example above abbreviates `definition` only to show the response envelope;
the API returns the complete stored nodes and edges. Workflow statuses are
`draft`, `published`, and `archived`.

Malformed, deleted, or cross-organization resources use the documented
tenant-safe error behavior. Authentication failures return `401`; rate-limit
exhaustion returns `429`. The shared request decoder caps complete JSON request
bodies at 278,528 bytes. Execution `input` itself is capped at 262,144 bytes
(256 KiB); oversized input returns `413 payload_too_large`.

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
      "position": {"x": 80, "y": 180},
      "config": {}
    },
    {
      "id": "is-paid",
      "type": "condition",
      "name": "Order paid?",
      "position": {"x": 340, "y": 180},
      "config": {
        "field": "input.status",
        "operator": "equals",
        "value": "paid"
      }
    },
    {
      "id": "send-receipt",
      "type": "task",
      "name": "Send receipt",
      "position": {"x": 620, "y": 80},
      "config": {
        "queue": "email",
        "payload": {"template": "receipt"},
        "priority": 5,
        "max_retries": 3,
        "timeout_seconds": 60
      }
    },
    {
      "id": "grace-period",
      "type": "delay",
      "name": "Payment grace period",
      "position": {"x": 620, "y": 300},
      "config": {
        "duration_seconds": 30
      }
    }
  ],
  "edges": [
    {
      "id": "order-to-condition",
      "from": "order-created",
      "to": "is-paid",
      "condition": null
    },
    {
      "id": "paid-to-receipt",
      "from": "is-paid",
      "to": "send-receipt",
      "condition": {"branch": true}
    },
    {
      "id": "unpaid-to-delay",
      "from": "is-paid",
      "to": "grace-period",
      "condition": {"branch": false}
    }
  ]
}
```

Draft ingestion enforces the JSON shape, supported node types, non-empty and
unique node/edge IDs, object-shaped node configs, and valid edge references.
Unknown fields and malformed definitions return `400 validation_failed`.
`position` is the only optional canvas presentation field and does not affect
graph validation or execution.

## Copy-paste API walkthrough

Set the local values once:

```bash
API_BASE="http://localhost:8080"
API_KEY="queueflow-dev-key"
```

### 1. Create a workflow

```bash
curl -i "$API_BASE/v1/workflows" \
  -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Order receipt workflow",
    "slug": "order-receipt-workflow",
    "description": "Send a receipt for paid orders or wait for payment.",
    "definition": {
      "nodes": [
        {
          "id": "order-created",
          "type": "trigger",
          "name": "Order created",
          "position": {"x": 80, "y": 180},
          "config": {}
        },
        {
          "id": "is-paid",
          "type": "condition",
          "name": "Order paid?",
          "position": {"x": 340, "y": 180},
          "config": {
            "field": "input.status",
            "operator": "equals",
            "value": "paid"
          }
        },
        {
          "id": "send-receipt",
          "type": "task",
          "name": "Send receipt",
          "position": {"x": 620, "y": 80},
          "config": {
            "queue": "email",
            "payload": {"template": "receipt"},
            "priority": 5,
            "max_retries": 3,
            "timeout_seconds": 60
          }
        },
        {
          "id": "grace-period",
          "type": "delay",
          "name": "Payment grace period",
          "position": {"x": 620, "y": 300},
          "config": {"duration_seconds": 30}
        }
      ],
      "edges": [
        {
          "id": "order-to-condition",
          "from": "order-created",
          "to": "is-paid",
          "condition": null
        },
        {
          "id": "paid-to-receipt",
          "from": "is-paid",
          "to": "send-receipt",
          "condition": {"branch": true}
        },
        {
          "id": "unpaid-to-delay",
          "from": "is-paid",
          "to": "grace-period",
          "condition": {"branch": false}
        }
      ]
    }
  }'
```

The response is `201` and includes the generated workflow `id`. Copy it into:

```bash
WORKFLOW_ID="replace-with-workflow-id"
```

### 2. Validate the stored draft

```bash
curl -i "$API_BASE/v1/workflows/$WORKFLOW_ID/validate" \
  -X POST \
  -H "Authorization: Bearer $API_KEY"
```

A valid graph returns:

```json
{"valid": true, "errors": []}
```

### 3. Publish version 1

```bash
curl -i "$API_BASE/v1/workflows/$WORKFLOW_ID/publish" \
  -X POST \
  -H "Authorization: Bearer $API_KEY"
```

### 4. List immutable versions

```bash
curl -i "$API_BASE/v1/workflows/$WORKFLOW_ID/versions" \
  -H "Authorization: Bearer $API_KEY"
```

### 5. Start the latest published version

Omitting `version` selects the latest snapshot. Use a new key for every
deliberate user run:

```bash
curl -i "$API_BASE/v1/workflows/$WORKFLOW_ID/executions" \
  -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Idempotency-Key: order-123-receipt-run-1" \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "order_id": "order_123",
      "customer_id": "customer_456",
      "status": "paid"
    }
  }'
```

To pin a historical snapshot, include `"version": 1`. The `202` response
contains `execution_id`; copy it into:

```bash
EXECUTION_ID="replace-with-execution-id"
```

### 6. List executions

```bash
curl -i \
  "$API_BASE/v1/workflows/$WORKFLOW_ID/executions?status=running&limit=50" \
  -H "Authorization: Bearer $API_KEY"
```

Remove `status=running` to list every status. Pass the opaque `next_cursor`
value unchanged to request the next page.

### 7. Inspect execution and node detail

```bash
curl -i \
  "$API_BASE/v1/workflows/$WORKFLOW_ID/executions/$EXECUTION_ID" \
  -H "Authorization: Bearer $API_KEY"
```

### 8. Cancel an active execution

```bash
curl -i \
  "$API_BASE/v1/workflows/$WORKFLOW_ID/executions/$EXECUTION_ID/cancel" \
  -X POST \
  -H "Authorization: Bearer $API_KEY"
```

Cancellation applies only while the execution is `pending` or `running`.

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

- include explicit `nodes` and `edges` arrays;
- use unique, non-empty node IDs and edge IDs;
- use only `trigger`, `task`, `webhook`, `delay`, and `condition` nodes;
- reference existing nodes from every edge;
- contain at least one trigger (multiple triggers are supported) and at least
  one task, webhook, delay, or condition node;
- keep trigger nodes free of incoming edges;
- make every non-trigger node reachable from a trigger;
- contain no isolated/orphan nodes;
- contain no cycles, self-loops, or duplicate source-to-target edges;
- use object-shaped node configs and object-or-null edge conditions.

Condition nodes may have multiple outgoing branches. Validation error objects
have stable `code`, `message`, and, where applicable, `path` fields.

Common validation codes include `nodes_required`, `edges_required`,
`node_id_required`, `duplicate_node_id`, `edge_id_required`,
`duplicate_edge_id`, `invalid_edge_reference`, `unsupported_node_type`,
`invalid_node_config`, `missing_trigger`, `missing_action`,
`trigger_has_incoming_edge`, `unreachable_node`, `orphan_node`, `self_loop`,
`duplicate_edge`, `invalid_condition_branch`, and `cycle_detected`.

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

## Architecture and durability

### Drafts, validation, and publishing

`workflows` stores the mutable tenant-scoped draft, including optional canvas
positions. Graph validation is deterministic and runs from the stored
definition. Publishing revalidates inside the use-case boundary and uses a
Postgres transaction that locks the workflow, assigns the next sequential
version, inserts the `workflow_versions` snapshot, and marks the workflow
published. The unique `(org_id, workflow_id, version)` constraint and publish
conflict handling protect concurrent publishes.

Version rows have no update API. Each snapshot contains the workflow name,
slug, description, and complete definition so historical interpretation does
not depend on the current draft.

### Executions and node records

Every execution references one immutable `workflow_versions` row. Postgres is
the source of truth for `workflow_executions` and
`workflow_node_executions`. Starting an execution creates its execution and
node rows transactionally. Each node record carries independent status,
attempt, timing, input/output, structured error, and optional QueueFlow task or
EventForge delivery linkage.

The producer runs a reconciliation loop, controlled by
`WORKFLOW_RECONCILE_INTERVAL_SEC` (default `1`). It scans `pending` and
`running` executions, determines runnable nodes from the immutable graph,
claims them with compare-and-set updates, observes external QueueFlow tasks and
EventForge deliveries, advances dependencies, and finalizes the execution.

Transactions cover execution creation, initial node rows, cancellation, and
terminal finalization. Node claims are short Postgres compare-and-set
transitions; QueueFlow enqueue and EventForge delivery creation occur outside
those database transactions. This avoids holding a database transaction across
an external dispatch.

Task nodes create durable Postgres QueueFlow tasks and use Redis-backed
priority transport. Webhook nodes create durable EventForge delivery records.
Task idempotency keys and webhook delivery UUIDs derive deterministically from
the execution/node identity. A duplicate reconciliation pass therefore
observes or reuses the same external work instead of dispatching a second copy.

Delay nodes persist `available_at`; the reconciler resumes them when due. No
worker goroutine sleeps for a workflow delay. Condition nodes read only the
execution input and deterministically select boolean edge metadata. Nodes on
unselected branches become `skipped` after dependency resolution.

If a process exits after claiming a node but before linking external work, the
claim is treated as in-flight for 30 seconds. After that stale-claim lease, the
reconciler retries the deterministic dispatch. This balances race avoidance
with crash recovery.

Execution statuses are `pending`, `running`, `succeeded`, `failed`, and
`cancelled`. Node statuses are `pending`, `queued`, `running`, `succeeded`,
`failed`, `skipped`, and `cancelled`.

## Starting and inspecting executions

Starting an execution requires `Idempotency-Key`. The key is unique within the
organization. Repeating the same key and canonical request returns the original
execution; reusing it for a different workflow, version, or input returns
`409 idempotency_conflict`. Separate deliberate user runs require separate
keys. The server stores the key and a canonical request fingerprint but never
returns either value in API responses.

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

Trigger nodes are entry points. They succeed immediately, receive the
execution input, and perform no external work. A workflow may have multiple
triggers, and triggers cannot have incoming edges.

Task nodes enqueue into QueueFlow. `queue` is required. `payload` is optional
and defaults to the execution input. `priority` is optional (`0..9`),
`max_retries` is optional (`0..99`), and `timeout_seconds` is optional
(`0..86400`, with `0` selecting the 60-second runtime default). Queue names
must contain 1–64 characters. QueueFlow owns retry, exponential backoff,
visibility timeout, and DLQ behavior.

```json
{
  "queue": "payments",
  "payload": {"kind": "charge"},
  "priority": 5,
  "max_retries": 3,
  "timeout_seconds": 60
}
```

Webhook nodes require an EventForge `endpoint_id` UUID. The endpoint must
exist in the same organization and be active when the node dispatches. Their
optional payload defaults to the execution input. EventForge performs signing,
delivery retries, and terminal failure handling with the existing public
headers: `X-Windylane-Event`, `X-Windylane-Delivery`,
`X-Windylane-Timestamp`, and `X-Windylane-Signature`. The event type is
`workflow.node`.

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

The dashboard surfaces this best-effort boundary before cancellation. A
cancelled timeline is the workflow orchestration state, not proof that an
already-dispatched external task or delivery stopped.

## Current boundaries

TaskCanvas deliberately does not yet provide:

- autosave, undo/redo, or collaborative editing;
- workflow replay or manual node retry;
- arbitrary code or an arbitrary condition language;
- live animated execution paths on the editor graph;
- QueueLens analytics or workflow throughput dashboards;
- advanced templates or rollback;
- a general-purpose distributed DAG scheduler beyond the durable TaskCanvas
  reconciler.

Cancellation prevents new downstream scheduling, but stopping a QueueFlow task
or EventForge delivery that already started is best-effort. The execution
timeline polls; it does not use a push stream.

## Troubleshooting

### Docker port conflicts

The default host ports are Postgres `5432`, Redis `6379`, and API `8080`.
Choose unused ports without changing container-to-container addresses:

```bash
POSTGRES_PORT=55433 REDIS_PORT=56380 API_PORT=58080 \
  docker compose up --build -d
```

Align the dashboard with the overridden API port:

```text
QUEUEFLOW_API_BASE_URL=http://localhost:58080
QUEUEFLOW_API_KEY=queueflow-dev-key
```

### Legacy pre-windylane containers

Detect containers that still use the old `flowforge-` project prefix:

```bash
docker ps -a --format '{{.Names}}' | grep '^flowforge-'
```

Inspect the output before changing anything. Stop and remove only explicit
container names:

```bash
docker stop CONTAINER_NAME
docker rm CONTAINER_NAME
```

Do not remove volumes unless their data is intentionally disposable.

### Migration failures

Check service state and migration logs:

```bash
docker compose ps -a
docker compose logs postgres migrate
docker network inspect windylane_default
```

Postgres and `migrate` must share `windylane_default`. First recreate the
containers while preserving the named database volume:

```bash
docker compose down --remove-orphans
docker compose up --build -d
```

`docker compose down -v` deletes the local Postgres volume. Use `-v` only when
all local data can be erased and a clean migration 1→8 is explicitly required.

### Dashboard API connectivity

- Verify `curl http://localhost:8080/healthz` or the overridden `API_PORT`.
- Confirm `QUEUEFLOW_API_BASE_URL` uses the same host port.
- Confirm `QUEUEFLOW_API_KEY=queueflow-dev-key` for the seeded local tenant.
- Keep the API key server-only; do not rename it with a `NEXT_PUBLIC_` prefix.
- A `401` indicates missing/invalid Bearer authentication; `429` indicates the
  tenant rate limit.

### Hydration warnings

Browser extensions can inject attributes such as `bis_register` before React
hydrates the page. Reproduce in an incognito/private window with extensions
disabled. Do not add blanket `suppressHydrationWarning`; fix application-owned
server/client markup only when the warning reproduces without extensions.

### Workflow validation failures

Common causes are a missing trigger, no executable/action node, an orphan or
unreachable node, a cycle, an incoming edge to a trigger, a self-loop, a
duplicate source/target edge, invalid references, missing condition branch
metadata, or invalid node configuration. Use each error's `code`, `message`,
and `path` to locate the exact definition element.

### Execution does not start

- Publish a valid workflow first; drafts return `409 workflow_not_published`.
- Confirm an explicit version exists on the same workflow and tenant.
- Send a non-empty `Idempotency-Key` no longer than 255 characters.
- Keep execution input at or below 256 KiB.
- For webhook nodes, use an existing active endpoint in the same organization.
- Revalidate task queues, delay bounds, condition fields/operators, and
  condition branch metadata.
- Reusing a key with changed content returns `409 idempotency_conflict`; create
  a new key for a separate run.
