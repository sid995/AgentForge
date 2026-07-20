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
