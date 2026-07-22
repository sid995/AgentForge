# Controller Behavior

## Reconcile sequence

1. Fetch object and handle not-found.
2. Handle deletion and finalization.
3. Validate defaults and desired state.
4. Ensure owned resources.
5. Observe Job and Pod conditions.
6. Classify state and failure.
7. Patch status only when changed.
8. Emit metrics and durable lifecycle command/event.
9. Requeue only when polling is required.

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
