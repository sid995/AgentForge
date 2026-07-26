# Runbook Catalog

Required runbooks:

- API elevated errors.
- PostgreSQL unavailable or pool exhaustion.
- Kafka unavailable or consumer lag.
- scheduler stalled.
- run queue delay high.
- controller reconcile failures.
- unschedulable Pods.
- AgentRun stuck without heartbeat.
- object-storage upload failures.
- registry build or pull failure.
- Argo CD OutOfSync or Degraded.
- TLS and DNS failures.
- cost anomaly.

Every runbook includes impact, symptoms, dashboards, queries, immediate mitigation, diagnosis, recovery, verification, escalation, and follow-up.

## Retained AgentRun cleanup blocked

- **Impact:** an AgentRun remains terminating while its explicitly retained PVC
  awaits handoff; execution is already stopping or stopped.
- **Symptoms:** `CleanupPending=True` with `RetainedWorkspaceReadFailed`,
  `RetentionHandoffFailed`, `RetainedWorkspaceConflict`, or
  `FinalizerRemovalFailed`; correlated Operator logs carry tenant, project,
  run, and attempt identifiers.
- **Dashboards and queries:** inspect controller reconcile error/rate metrics,
  then `kubectl get agentrun <name> -n <namespace> -o yaml` and list PVCs with
  `execution.agentforge.dev/run-id=<run-id>`. Never print Secret data.
- **Immediate mitigation:** restore Kubernetes API/RBAC availability or remove
  the conflicting external metadata mutation. Do not delete a retained PVC.
- **Diagnosis:** verify the PVC has no controller owner and its
  `owner-uid`/`retention-policy` annotations match the terminating AgentRun.
  Check whether earlier attempt PVCs already have `retention-state: Released`.
- **Recovery:** allow the 15-second reconciliation retry to mark every retained
  attempt PVC `Released` and remove the finalizer. If an externally managed
  controller owner was added, remove it only after confirming the retention
  request and object identity.
- **Verification:** the AgentRun is absent; every retained PVC remains present
  and carries `retention-state: Released`; no PVC delete event was issued by
  the Operator.
- **Escalation:** after ten minutes the Operator records `CleanupEscalated`,
  releases the finalizer without deleting storage, and logs operator-review
  context. Inventory PVCs by owner UID/run ID and open an incident if a handoff
  marker is missing or ownership remains inconsistent.
- **Follow-up:** correct the RBAC, admission, or external mutation source and
  attach bounded condition/log evidence to the incident; never attach raw
  workspace contents.
