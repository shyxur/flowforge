# ADR 0002: Bounded non-blocking metric recording

Status: Accepted

## Context

QueueLens observes task, delivery, workflow, node, and worker lifecycles.
Metric persistence can become slow or unavailable independently of the primary
operation.
Unbounded buffering risks uncontrolled memory growth, while synchronous writes
extend business-operation latency and failure scope.

## Decision

Metric events enter a bounded process-local buffer through a non-blocking
recorder.
A background loop flushes events to PostgreSQL in bounded batches.
When the buffer is full, new metric events are dropped and counted.
Persistence errors are logged without failing task execution, webhook delivery,
or workflow execution.

## Consequences

Primary operations continue when metric storage is slow or unavailable.
Memory use and flush work remain bounded.
Temporary metric loss is accepted as preferable to blocking the primary
system.
Buffer capacity and flush settings require explicit operational tuning.
