# ADR 0004: Immutable workflow versions

Status: Accepted

## Context

TaskCanvas users need to edit workflows without changing definitions already
used by active or historical executions.
Execution timelines must retain the node labels, graph, and configuration that
were published for that run.
Reading a mutable definition would make historical behavior irreproducible.

## Decision

Workflow drafts remain mutable until publication.
Publishing creates a new immutable workflow version rather than modifying an
existing published version.
Each execution records and reads its selected version snapshot.
Later edits apply only to the draft used for a future publication.

## Consequences

Historical execution timelines remain consistent with their original graph and
node configuration.
Active executions cannot be mutated by later workflow edits.
Future revisions can be prepared safely while older versions remain readable.
Published-version storage grows with each release and requires explicit version
selection and retention policy decisions.
