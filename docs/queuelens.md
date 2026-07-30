# QueueLens metrics foundations

QueueLens is windylane's observability phase for QueueFlow, EventForge, and
TaskCanvas. M1 provides a private, durable metric-event foundation and
low-impact runtime instrumentation. It does not expose an analytics API or add
dashboard charts.

## Architecture

Runtime transitions create validated `MetricEvent` values after their durable
business-state changes complete. A process-local producer places them on a
bounded channel without waiting. A background loop writes batches to Postgres
outside business transactions:

```text
durable lifecycle transition
    -> validated, deterministic MetricEvent
    -> bounded non-blocking channel
    -> batch/time-based flush
    -> append-only Postgres metric_events
```

The buffer holds at most `METRICS_BUFFER_CAPACITY` events. When full, new
events are dropped and counted rather than slowing task, delivery, or workflow
execution. Batch write failures are logged with only an event count and error;
they are not returned to the business operation. Shutdown stops new recording,
drains the bounded buffer, and performs a final flush with a timeout.

Each event ID is a UUID derived from organization, source, event type, resource
type, resource ID, and a transition key such as an attempt number. Reprocessing
the same transition therefore produces the same ID. The table primary key and
`ON CONFLICT DO NOTHING` make inserts idempotent.

Postgres remains the durable metrics source. Redis is not used to store or
transport QueueLens events.

## Event model

Every event contains:

- `id`, `organization_id`;
- `source`, `event_type`, `resource_type`, `resource_id`;
- optional `queue`, `status`, and non-negative `duration_ms`;
- `occurred_at` and `created_at`;
- closed, bounded metadata: `attempt`, `max_attempts`, `error_code`, and
  `previous_status`.

Metadata is a typed object, not an arbitrary label map. Validation limits
resource IDs to 255 bytes, queue/status/error categories to 64 bytes, and
serialized metadata to 512 bytes in Go. Postgres additionally limits the JSON
object to 2 KiB.

Metric records never include API keys, authorization or signature headers,
webhook secrets or URLs, idempotency keys, request/response bodies, task
payloads, workflow inputs, personal data, or raw error messages. `error_code`
stores only a controlled category such as `handler_error` or
`delivery_error`.

## Sources and event types

| Source | Resource | Events |
| --- | --- | --- |
| `queueflow` | `task` | `task.ingested`, `task.started`, `task.succeeded`, `task.failed`, `task.retry_scheduled`, `task.dead_lettered`, `task.cancelled` |
| `eventforge` | `webhook_delivery` | `delivery.created`, `delivery.started`, `delivery.succeeded`, `delivery.failed`, `delivery.retry_scheduled`, `delivery.exhausted` |
| `taskcanvas` | `workflow_execution` | `workflow_execution.created`, `workflow_execution.started`, `workflow_execution.succeeded`, `workflow_execution.failed`, `workflow_execution.cancelled` |
| `taskcanvas` | `workflow_node_execution` | `node_execution.started`, `node_execution.succeeded`, `node_execution.failed`, `node_execution.skipped`, `node_execution.cancelled` |
| `worker` | `worker` | `worker.registered`, `worker.heartbeat`, `worker.stopped`, `worker.stale` |
| `queue` | `queue_snapshot` | `queue.snapshot.captured` |

`worker.stale` and `queue.snapshot.captured` are validated foundation event
types for later QueueLens collectors. M1 runtime instrumentation emits worker
registration, heartbeat, and clean-stop events; it does not yet run a stale
worker or queue-snapshot collector.

Task metrics are emitted after ingestion, processing claim notification,
successful completion, durable retry/DLQ updates, and cancellation. EventForge
records delivery creation, each claimed attempt, delivery outcome, retry
scheduling, and exhaustion. TaskCanvas records execution and node transitions
after successful CAS or finalization updates, so reconciliation cannot create
additional rows for the same transition.

## Storage and queries

Migration `0009_queuelens_metrics` adds only `metric_events`. It is
organization-scoped through a required foreign key, and has indexes beginning
with `org_id` for:

- reverse chronological time-range queries;
- source and event-type filtering;
- resource history filtering.

An update/delete trigger enforces append-only use. Query access is currently
internal through `MetricsService` and `MetricRepository`; no
`/v1/metrics/events` route is exposed in M1.

Internal queries require:

- a non-zero organization ID;
- `from` and `to`, with `from < to`;
- a time range no longer than 31 days;
- a page size from 1 through 1,000;
- `source` when `event_type` is supplied.

Results use an opaque cursor and deterministic
`occurred_at DESC, id DESC` ordering. Organization scope is always the first
query predicate.

## Configuration

| Environment variable | Default | Valid range |
| --- | ---: | ---: |
| `METRICS_ENABLED` | `true` | boolean |
| `METRICS_BUFFER_CAPACITY` | `2048` | 1–100,000 |
| `METRICS_BATCH_SIZE` | `100` | 1–1,000 and no larger than the buffer |
| `METRICS_FLUSH_INTERVAL_SEC` | `1` | 1–60 seconds |
| `METRICS_WRITE_TIMEOUT_SEC` | `5` | 1–60 seconds |

Invalid enabled-producer settings fail process startup before external
connections are used. With `METRICS_ENABLED=false`, no producer goroutine is
started and QueueFlow, EventForge, and TaskCanvas behavior is unchanged.

## Operations and troubleshooting

- Metric rows absent: confirm migration 0009 ran and `METRICS_ENABLED=true` in
  each producer, QueueFlow worker, and EventForge worker process.
- Intermittent gaps: inspect `metric batch write failed` logs and check
  Postgres connectivity. The original business operation is intentionally not
  retried or failed because of metrics.
- Sustained gaps under load: increase the bounded capacity or batch size within
  their limits, or reduce the flush interval. Producer drops are preferable to
  unbounded memory growth.
- Duplicate transition observed in runtime logs: storage still deduplicates the
  deterministic event ID. Distinct attempts and heartbeats are distinct
  transitions by design.
- Query rejected: use a range no longer than 31 days and page size no larger
  than 1,000. Event-type filters must include their source.

## M1 limits

M1 has no public analytics endpoint, dashboard visualization, percentile
calculation, time rollups, retention cleanup, error grouping, DLQ explorer,
Prometheus/OpenTelemetry exporter, alerting, billing metrics, stale-worker
collector, or queue-snapshot collector. It stores raw bounded lifecycle events
only; later QueueLens milestones will build aggregation and presentation on
this foundation.
