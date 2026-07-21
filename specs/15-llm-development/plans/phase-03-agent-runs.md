# Phase 3 Plan: AgentRun Domain and State Machine

## Status

In progress. Prompt 3.1 state-machine normalization was committed in `5c4a8fd`.
Prompt 3.2 adds the durable persistence foundation; later prompts add the
authenticated API, cancellation command, retry command, and completion audit.

## Implemented persistence boundary

Phase 3.2 persists tenant-scoped `AgentRun` and `AgentRunAttempt` aggregates
with application-generated UUIDv7 IDs, explicit status/failure value objects,
optimistic versions, secure prompt references, bounded runtime/resource/timeout
input, monotonic attempt numbers, and tenant-scoped idempotency-key uniqueness.
Run creation atomically inserts its first pending attempt. Repository reads,
lists, and writes are tenant transactions with explicit predicates and forced
PostgreSQL RLS. A stale run or attempt write maps to a stable version conflict.

This sub-phase deliberately excludes HTTP identity, API routes, scheduling,
workload creation, audit persistence, transactional outbox rows, and Kafka.
Those behaviours are implemented only in their later documented sub-phases.

## Acceptance evidence

- Domain tests cover valid and prohibited run/attempt transitions, retry guards,
  terminal state handling, and bounded input validation.
- PostgreSQL integration tests apply migrations, create and list runs/attempts,
  enforce idempotency uniqueness and RLS isolation, reject stale attempt writes,
  and prove concurrent cancellation writes cannot silently overwrite one
  another.
