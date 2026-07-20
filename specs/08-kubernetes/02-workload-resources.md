# Execution Workload Resources

For each attempt, the operator ensures:

- deterministic Job.
- restricted ServiceAccount with token automount disabled unless required.
- ConfigMap for non-sensitive configuration.
- projected or brokered short-lived credentials.
- PVC or ephemeral volume for workspace.
- default-deny NetworkPolicy plus explicit egress.
- labels for tenant, project, run, attempt, runtime, and workload class.
- active deadline and termination grace period.

## Scheduling

Agent and build workloads use dedicated tainted node pools. Use priority classes, topology spread, and quotas. Never place untrusted workloads on control-plane nodes.
