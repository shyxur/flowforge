# ADR 0003: Deterministic metric deduplication

Status: Accepted

## Context

Retries, recovery, reconciliation, and repeated lifecycle observation can
produce the same logical metric event more than once.
Process-local duplicate detection cannot protect across restarts or multiple
runtime processes.
Metric insertion must remain safe to repeat.

## Decision

Each metric event receives a deterministic UUID derived from stable transition
identity inputs.
PostgreSQL stores that UUID as the primary key.
Inserts use `ON CONFLICT DO NOTHING`.
The database therefore enforces duplicate protection while callers may repeat
an append safely.

## Consequences

Retries and recovery produce idempotent metric insertion.
Duplicate protection works across processes and restarts.
Event identity inputs must remain stable and carefully defined.
Changing identity inputs can split one logical transition into multiple events
or merge transitions that should remain distinct.
