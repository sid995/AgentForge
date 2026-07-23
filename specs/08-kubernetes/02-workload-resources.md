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

## Phase 6.4 resource builders

Pure builders under `operator/internal/resources` produce one deterministic
ServiceAccount, immutable runner ConfigMap, workspace PVC, NetworkPolicy, and
Job for the current observed attempt. They perform no API writes. Every
cluster-local child uses an AgentRun controller owner reference, except a PVC
with explicit `Retain` policy; retained PVCs omit garbage-collection ownership
and carry a retention annotation for finalizer cleanup.
Phase 6.9 preserves those claims after AgentRun deletion and adds
`execution.agentforge.dev/retention-state: Released` only after validating the
claim's retention and owner-UID markers. No retained-workspace cleanup path
issues a PVC delete.

The Job uses a digest-qualified runner image, active deadline, zero Kubernetes
backoff, exact CPU/memory requests and limits, bounded termination grace, and
one deterministic `/workspace` PVC mount. Non-sensitive runtime configuration
is immutable. External ConfigMaps and Secrets are sorted and mounted read-only
at bounded paths; secret values are never copied into the CR, ConfigMap,
annotations, or environment variables. The artifact destination is referenced
as a ConfigMap volume.

Tenant-controlled desired state selects only a trusted execution profile.
Profiles must select `agentforge.dev/node-pool=agents`, cannot tolerate or
select control-plane nodes, and may add validated tolerations, priority class,
topology spread keys, and runtime class. The standard profile spreads across
zone and hostname when available. Builder configuration is deep-copied so
later caller mutation cannot weaken a running builder.

The NetworkPolicy always selects one run and attempt, denies all ingress, and
governs egress. `Isolated` has no egress rules. `RestrictedEgress` copies only
a matching trusted policy containing explicit peers and TCP/UDP ports. Empty
selectors, wildcard or excessively broad CIDRs, link-local ranges, and cloud
metadata destinations are rejected before a manifest is returned.
