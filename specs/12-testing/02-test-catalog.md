# Required Test Catalog

## Run lifecycle

- Duplicate create command.
- Invalid transition.
- cancellation races completion.
- timeout during model call.
- scheduler crash before and after CR creation.
- operator restart during active Job.
- retryable and terminal attempt failure.
- concurrent Scheduler claims, active/expired leases, priority and age order,
  fairness rounds, optimistic renewal conflicts, empty queues, transient
  database errors, and Scheduler-role tenant/data isolation.
- eligibility outcome/explanation coverage at tenant/user concurrency, queue,
  CPU, memory, budget, runtime/profile, and tenant/project-status boundaries;
  concurrent decision persistence and noisy-neighbour tenant isolation.
- cluster metadata validation, freshness/maintenance/runtime/allowlist filters,
  deterministic strategy ties and regional fallback, concurrent capacity and
  budget reservation, idempotent release/settle/reclaim, atomic assignment and
  outbox replay/rollback, policy rejection, worker shutdown, health/readiness,
  metrics, and backpressure bounds.

## Messaging

- duplicate delivery.
- out-of-order aggregate version.
- publish failure after transaction.
- consumer crash after commit before acknowledgment.
- DLQ and authorized replay.
- canonical examples and typed producer output against machine-readable schemas.
- immutable-major required-field and JSON-type compatibility baselines.
- supported old consumer fixtures against current schemas and runtime decoders.
- retry-attempt progression and publish-before-acknowledgment against Redpanda.

## Isolation

- cross-tenant API and repository access.
- RLS enforcement.
- object-storage prefix isolation.
- Kubernetes namespace and egress isolation.

## Kubernetes controller

- reproducible CRD, RBAC, and deepcopy generation with pinned tools;
- manager scheme registration for `execution.agentforge.dev/v1alpha1`;
- envtest installation and create/get behavior for the AgentRun CRD;
- non-root Operator image and configured leader election, health, readiness,
  authenticated metrics, and structured logging foundation.

## Deployment

- competing promotions.
- invalid manifest.
- Argo degraded rollout.
- rollback to prior digest.
