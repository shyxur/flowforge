# ADR 0001: PostgreSQL and Redis

Status: Accepted

## Context

windylane needs durable lifecycle state and efficient worker-facing queue
coordination.
PostgreSQL provides transactions, recovery, and durable records for tasks,
deliveries, workflows, executions, and metrics.
Redis provides priority-aware dispatch, delayed and processing sets, queue
coordination, and worker-facing hot state.

## Decision

PostgreSQL is the durable source of truth.
Redis is an operational dispatch and coordination layer, not the authoritative
database.
No durable lifecycle state may depend exclusively on Redis.
Reconciliation rebuilds or repairs Redis state from PostgreSQL when needed.

## Consequences

The system can recover durable state after Redis loss or dispatch gaps.
Worker dispatch avoids using PostgreSQL as the only hot queue mechanism.
Operating two data systems adds deployment, monitoring, and failure-handling
complexity.
