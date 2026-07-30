# ADR 0005: Layered tenant isolation

Status: Accepted

## Context

windylane stores and dispatches data for multiple organizations.
A missed tenant check at any single boundary could expose data or route work to
the wrong organization.
Authentication alone cannot constrain repository queries, Redis keys, or
worker dispatch after a request enters the system.

## Decision

Tenant identity is enforced independently by API authentication, application
use cases, repository query predicates, and PostgreSQL records.
Redis keys and queue operations include organization scope.
Broker messages, claims, worker pools, and dispatch operations preserve the
organization boundary.
Tests cover cross-tenant reads and lifecycle operations at these layers.

## Consequences

No single enforcement point is trusted as the complete isolation boundary.
Repeated checks provide deliberate defense in depth.
The approach adds implementation and testing cost across APIs, storage,
brokers, and workers.
New features must carry tenant scope through every relevant boundary.
