# Agent Kubernetes Operator

## CRDs

- `AgentRun`: desired execution workload and observed status.
- Optional later CRDs: `AgentPool`, `SandboxProfile`.

## Reconcile responsibilities

- Validate specification.
- Add finalizer.
- Ensure service account, workspace, configuration, policy, and Job.
- Observe Pod and Job state.
- Classify failure.
- Update status conditions.
- Emit metrics and lifecycle events.
- Clean external resources before removing finalizer.

## Controller rules

- Reconciliation is idempotent.
- Do not depend on a previously executed step being remembered in memory.
- Use owner references for cluster-local resources.
- Use finalizers only for external cleanup.
- Requeue with bounded exponential backoff.
- Limit status payload size and avoid high-frequency writes.
