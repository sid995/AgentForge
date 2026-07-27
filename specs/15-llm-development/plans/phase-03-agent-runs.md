# Phase 3 Plan: AgentRun Domain and State Machine

## Status

Phase 3.1 state-machine normalization was committed in `5c4a8fd`; Phase 3.2
adds the durable persistence foundation. Phase 3.3 implementation is complete:
the Platform
API has authenticated create/get/project-list routes, server-derived temporary
development identity, canonical request hashing and idempotent replay, opaque
cursor pagination, an executable OpenAPI contract, HTTP contract tests, and a
PostgreSQL application integration test. Unit, race, vet, OpenAPI contract,
and repository verification pass. PostgreSQL integration passes; the lint gate
remains unverified because its pinned container image pull stalled.
Phase 3.4/3.5 are implemented in `db6eed6`: authenticated commands persist
tenant-scoped durable cancellation/retry state, a safe latest-command receipt,
monotonic retry attempts, and paired versioned outbox events. They intentionally
do not contact Kubernetes or project Operator status into PostgreSQL. The Phase
3 completion audit remains implemented but unverified until the lint gate is
available.

## Implemented persistence boundary

Phase 3.2 persists tenant-scoped `AgentRun` and `AgentRunAttempt` aggregates
with application-generated UUIDv7 IDs, explicit status/failure value objects,
optimistic versions, secure prompt references, bounded runtime/resource/timeout
input, monotonic attempt numbers, and tenant-scoped idempotency-key uniqueness.
Run creation atomically inserts its first pending attempt. Repository reads,
lists, and writes are tenant transactions with explicit predicates and forced
PostgreSQL RLS. A stale run or attempt write maps to a stable version conflict.

Phase 3.2 deliberately excluded HTTP identity and API routes. Phase 3.3 adds a
temporary server-configured opaque Bearer identity only in `development` and
`test`; it never trusts a client tenant field/header and is explicitly replaced
by Phase 12 OIDC/RBAC. The create path calls the existing atomic
run/attempt/outbox repository transaction; it does not schedule or execute a
workload. Audit persistence remains deferred.

## Acceptance evidence

- Domain tests cover valid and prohibited run/attempt transitions, retry guards,
  terminal state handling, and bounded input validation.
- PostgreSQL integration coverage applies migrations and verifies create,
  durable cancellation/retry receipt replay, monotonic attempt insertion, and
  paired outbox event types when the integration database is available.
