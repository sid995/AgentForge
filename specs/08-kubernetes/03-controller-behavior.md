# Controller Behavior

## Reconcile sequence

1. Fetch object and handle not-found.
2. Handle deletion and finalization.
3. Validate defaults and desired state.
4. Ensure owned resources.
5. Observe Job and Pod conditions.
6. Classify state and failure.
7. Patch status only when changed.
8. Emit metrics; publish a durable lifecycle fact only after its later
   cross-store contract is implemented.
9. Requeue only when polling is required.

Phase 6 ends at Kubernetes status ownership. The Operator does not write
PostgreSQL workflow state, and no observed-lifecycle producer is claimed by
the Phase 6 completion gate.

## Failure classification

- retryable infrastructure: eviction, temporary pull error, scheduling capacity, transient storage.
- terminal execution: tests failed, invalid output, policy rejection.
- platform defect: malformed owned resource, invariant violation.

## Tests

Use controller-runtime envtest for reconciliation and kind for integration behavior including owner references, finalizers, deletion, retry, and restart recovery.

## Phase 6.3 reconciliation foundation

The `AgentRunReconciler` is registered with the manager and watches creates,
spec-generation changes, and the start of deletion. Status-only and
finalizer-only updates are filtered to prevent self-triggered loops. Each
reconcile fetches current API state, treats not-found as success, validates the
defense-in-depth execution contract, derives deterministic attempt resource
names, and writes status only when semantic state changed.

Valid new objects initialize to `Pending`, copy the current attempt and
namespace, set `observedGeneration`, and publish standard `SpecValid` and
`Ready` conditions. Invalid objects receive bounded validation diagnostics and
a terminal controller error. Status patches use optimistic resource-version
locking; conflicts and unknown transport errors remain transient. Forbidden,
unauthorized, invalid, bad-request, and unsupported operations are permanent
and use controller-runtime terminal errors.

The work queue uses per-object exponential retry starting at one second and
capped at two minutes. Reconciliation has a 30-second timeout and four bounded
workers. Resource names use the immutable compact run UUID plus attempt and
are stable across controller restarts.

The finalizer `execution.agentforge.dev/retained-resources` is added only when
the workspace explicitly requests `Retain`. Ordinary cluster-local ownership
does not receive a finalizer. During deletion, retained objects expose a
`CleanupPending=True` condition and remain for the Phase 6.9 cleanup contract.
Phase 6.3 creates no child resources.

## Phase 6.5 prerequisite reconciliation

The reconciler now resolves a trusted resource builder and ensures execution
prerequisites in a fixed order: tokenless ServiceAccount, same-namespace
ConfigMap and Secret references plus the owned immutable runner ConfigMap,
workspace PVC, deny-by-default NetworkPolicy, then Job. Ordinary pending PVCs
produce a five-second polling requeue before network or compute creation.
`WaitForFirstConsumer` storage is detected from the trusted StorageClass and
permits Job creation because scheduling the consumer is required to bind the
claim; the workspace remains explicitly pending until binding completes.

Child writes use server-side apply with field manager `agentforge-operator`
and never force ownership. Existing objects must already belong to the same
AgentRun (or carry the retained PVC owner-UID marker), and matching managed
state is a no-op. Externally owned labels and annotations are preserved.
Mismatched ownership and server-side-apply field conflicts are terminal,
bounded `CONFLICT` diagnostics rather than silent adoption or forced
overwrite.

`ServiceAccountReady`, `ConfigurationReady`, `WorkspaceReady`,
`NetworkPolicyReady`, and `JobReady` conditions expose the exact blocked
prerequisite. Missing configuration references requeue without creating later
resources. API transport and availability failures retain transient retry
classification; invalid or forbidden writes are permanent. Owned-resource
watches provide event-driven reconciliation while status patches remain
semantic and optimistic-lock protected.

## Phase 6.6 Job and Pod lifecycle

After deterministic Job creation, status is derived only from observed Job and
Pods whose controller owner UID matches that Job; label-only spoofed Pods are
ignored. The projector distinguishes unscheduled, scheduled,
container-creating, active, successful, and failed states. Stable bounded
reasons classify eviction, node loss, image-pull failure, invalid images,
container configuration/start failures, deadline expiry, OOM termination, and
nonzero process exit without copying raw Kubernetes messages into status.

`TRANSIENT_DEPENDENCY` marks eviction, node loss, and ordinary image-pull
failures; invalid image and container start failures are
`PERMANENT_DEPENDENCY`; rejected container configuration is `POLICY`; process,
memory, deadline, and result failures are `EXECUTION`. Phase 6.8 consumes this
taxonomy for retry decisions.

Runner exit zero is necessary but insufficient for success. The runner must
write a bounded strict JSON termination message with `schemaVersion: 1` and a
valid `artifactManifestRef`. The Operator never projects raw termination text.
Only a completed Job plus valid evidence becomes `Succeeded`; missing or
invalid evidence becomes `ResultEvidenceInvalid`.

Current Job and Pod names, start/completion timestamps, normalized failure,
artifact reference, conditions, and one bounded attempt-history entry are
projected with semantic optimistic status patches. Terminal current-attempt
status is stable across repeated reconciliation. If an already observed Job is
deleted, the attempt becomes `INTERNAL`/`JobMissing` and the same Job is not
recreated. Active states poll every five seconds, while owned Job events also
enqueue reconciliation.

## Phase 6.7 cancellation

One-way cancellation intent is reconciled before any new prerequisite can be
created. No-Job cancellation completes immediately. Otherwise the Operator
foreground-deletes the deterministic Job so dependent Pods receive their
configured graceful termination interval and the Job remains observable until
its dependents are gone.

The first `CancellationComplete=False` transition is the durable cancellation
clock. Repeated reconciliation and controller restart preserve its transition
time. Graceful termination is bounded to two minutes; after the deadline, only
Pods with an exact Job controller name and UID match receive zero-grace delete,
followed by a forced Job delete. Absence then projects terminal `Cancelled`
with either `GracefulCancellation` or `ForcedCancellation`. Repeated terminal
reconciliation is a semantic no-op, existing diagnostics are retained, and a
fully observed terminal Job outcome wins a later cancellation request.

## Phase 6.8 retry

A failed attempt retries only when its category is both present in the CR
allowlist and one of the platform-approved `TRANSIENT_DEPENDENCY` or `INTERNAL`
categories, with an observed attempt below `maxAttempts`. Security policy,
invalid configuration, permanent dependency, execution/test, success, and
cancellation states cannot retry.

Retry delay doubles per Operator attempt, stops at the CR maximum, and uses
deterministic 50-100% jitter from run identity plus next attempt. The terminal
completion time, or a stable condition transition when completion time is
absent, anchors the deadline across restarts. `RetryReady=False/BackoffPending`
is durable and requeues for the remaining interval.

After the deadline, one status patch retains the failed attempt and advances
the current attempt to `Pending`, clearing only current execution projections.
A subsequent reconciliation derives a new attempt-qualified Job and related
resources. This ordering prevents a status conflict or controller crash from
creating work for an uncommitted attempt, and the failed Job remains distinct.

## Phase 6.9 finalization and cleanup

Cluster-local disposable resources continue to rely on AgentRun controller
owner references. Only explicitly retained workspaces carry the
`retained-resources` finalizer. Deletion iterates from the Scheduler starting
attempt through the current observed attempt, tolerates absent claims, validates
unowned owner-UID/retention markers, and idempotently records a `Released`
handoff annotation. The Operator never deletes a retained PVC.

`CleanupPending=True` records sanitized read, ownership, patch, status, and
finalizer-removal blockers. Partial success persists on each PVC, and a
15-second explicit requeue makes transient errors and controller restart safe.
The deletion timestamp is the durable clock for a ten-minute escalation
deadline. Expiry records `CleanupEscalated`, releases the finalizer without a
PVC delete, and emits correlated operator-review telemetry; patch failures
continue polling rather than terminating the reconcile key.

## Phase 6.10 Scheduler intent projection

The Operator does not consume scheduling events. The separate handoff
component projects a validated `agent-run.scheduled.v1` fact into the selected
cluster as a deterministic AgentRun and never creates controller children.
It creates the object without status; the status subresource remains
Operator-exclusive. Exact existing specs are idempotent no-ops, while
mismatched immutable intent is a permanent conflict and is never overwritten.
The handoff validates a one-to-one cluster-context map before becoming ready,
bounds Kubernetes processing time, and isolates primary and delayed-retry
consumption. The Operator checks referenced Secrets through metadata-only,
uncached reads with get-only RBAC so Secret contents are never cached.
