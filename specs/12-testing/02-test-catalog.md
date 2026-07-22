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
- AgentRun required schema, safe defaults, enum/range/reference validation,
  prohibited resource/network/retry combinations, set-list uniqueness,
  immutable execution intent, one-way cancellation, printer columns, and
  status-subresource isolation against Kubernetes 1.36 envtest;
- reconciler not-found, create, generation update, conditional finalizer,
  deletion timestamp, optimistic status conflict/retry, duplicate reconcile,
  invalid-spec terminal classification, deterministic names, bounded
  exponential backoff and self-update predicate suppression;
- golden isolated ServiceAccount/ConfigMap/PVC/NetworkPolicy/Job manifests;
  trusted profile placement, tolerations, topology spread and runtime class;
  exact resources/deadline/backoff/workspace/secret mounts; retained PVC
  ownership; restricted-egress policy validation; configuration aliasing; and
  mutations for privilege, escalation, writable root, host namespaces,
  Kubernetes tokens, hostPath, Docker socket, and wildcard egress;
- ordered server-side apply of ServiceAccount, configuration, and PVC;
  matching-resource no-op behavior; preservation of external metadata;
  missing-reference and storage-pending requeues; PVC-bound NetworkPolicy and
  Job creation; conflicting pre-existing ownership; and prerequisite status
  conditions against Kubernetes 1.36 envtest;
- Job/Pod pending, scheduled, unschedulable, container-creating, active,
  succeeded, failed, deadline, OOM, eviction, node-loss, image-pull,
  configuration, nonzero-exit, missing-evidence, and missing-Job projections;
  ownership-filtered Pod observation; strict result-evidence parsing; bounded
  attempt history; terminal no-ops;
  `WaitForFirstConsumer` binding; and no duplicate Job creation;
- cancellation before Job creation, while pending, while running, after
  observed success, repeated cancellation, already-missing Job convergence,
  graceful delete, forced deadline, exact Job-owner Pod filtering, and
  controller restart recovery;
- retry allowlist intersection, every approved and prohibited category,
  attempt ceiling, deterministic exponential capped jitter, restart recovery,
  prior-attempt retention, current-field reset, new deterministic Job identity,
  failed-Job retention, and Kubernetes 1.36 envtest Job creation;
- a pinned kind 0.32.0 two-node cluster using Kubernetes 1.36.1 by digest,
  proving real scheduling, PVC binding, Pod completion, missing-evidence
  failure, attempt projection, one-Job idempotency, and a transient image-pull
  failure advancing to a distinct second-attempt Job;
- non-root Operator image and configured leader election, health, readiness,
  authenticated metrics, and structured logging foundation.

## Deployment

- competing promotions.
- invalid manifest.
- Argo degraded rollout.
- rollback to prior digest.
