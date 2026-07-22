# AgentRun CRD Specification

## API identity and ownership

`AgentRun` is a namespaced `execution.agentforge.dev/v1alpha1` resource with
short name `arun`. The Scheduler owns desired state and creates the resource
without a `status` field. The Operator exclusively writes the status
subresource. PostgreSQL remains authoritative for platform workflow state;
this CR records desired execution intent and observed Kubernetes state.

## Desired-state contract

The required identity is `tenantId`, `projectId`, `runId`, `attemptId`, and the
one-based `attempt`; identifiers are lowercase UUIDv7 values. Required
execution configuration is an immutable digest-qualified `runnerImage`,
normalized `runtime` and `executionProfile`, secure `taskRef`, bounded timeout,
retry policy, resource requests and limits, workspace, network profile, and
artifact destination reference. Optional configuration and secret references
are unique name-keyed lists. Secret values, prompts, and source content are
never stored in the CR.

`desiredState` defaults to `Running` and is the only ordinary mutable spec
field. It may transition to `Cancelled` but cannot return to `Running`.
`deployOnSuccess` defaults to false and records the later governed deployment
request; it does not authorize the Operator to deploy application workloads.

The schema applies these safe defaults:

- retry backoff starts at 5 seconds and is capped at 300 seconds;
- workspace retention is `Delete`;
- network profile is `Isolated`;
- desired state is `Running` and deployment-on-success is false.

## Validation and prohibited combinations

- IDs must be UUIDv7 and local references must be DNS subdomain names.
- The runner image must use an immutable SHA-256 OCI digest.
- Attempts are 1 through 10 and cannot exceed `retryPolicy.maxAttempts`.
- Timeout is 1 through 86,400 seconds; retry backoffs are bounded and the
  maximum cannot be below the initial value.
- Retry categories in desired state are limited to
  `TRANSIENT_DEPENDENCY` and `INTERNAL`; the controller applies the narrower
  runtime policy before retrying.
- CPU and memory have explicit ceilings, and every limit must be at least its
  corresponding request.
- `RestrictedEgress` requires `allowedDestinationsRef`; `Isolated` prohibits
  that reference.
- Execution identity, configuration, policy, and object references are
  immutable after creation.

Privileged execution settings are intentionally not exposed in this API. The
Operator derives security contexts and child resources from approved profiles.

## Observed-state contract

Status contains phase, observed generation, current attempt, namespace, Job
and Pod names, start/completion/heartbeat timestamps, bounded failure category
and reason, secure artifact-manifest reference, bounded attempt history, and
Kubernetes-standard conditions. Phase is one of `Pending`, `Provisioning`,
`Running`, `Succeeded`, `Failed`, `Cancelling`, or `Cancelled`. Printer columns
show phase, attempt, Job, and age.

The immutable spec `attempt` is the Scheduler-assigned starting execution
attempt for this CR. Approved Operator infrastructure retries advance only the
controller-owned status `attempt`, retain each prior status entry, and remain
bounded by `retryPolicy.maxAttempts`; they do not mutate Scheduler intent or
the immutable `attemptId`.

Failure categories are stable and non-sensitive: `VALIDATION`,
`AUTHENTICATION`, `AUTHORIZATION`, `QUOTA`, `CONFLICT`,
`TRANSIENT_DEPENDENCY`, `PERMANENT_DEPENDENCY`, `EXECUTION`, `POLICY`, and
`INTERNAL`. Human-readable diagnostics are length-bounded and must not contain
secrets or unrestricted workload content.

## Evolution and conversion strategy

`v1alpha1` is both the served and storage version while it is the only version.
Compatible alpha changes may add optional fields or safe defaults but must not
reinterpret existing fields. Before serving a second version with semantic or
shape differences, add conversion tests and a conversion webhook, select one
documented storage/hub version, preserve round-trip data, and provide rollout,
storage-migration, and rollback instructions. Removing fields or tightening
validation for already-stored objects requires an explicit compatibility and
migration plan.

## Implementation status

Phase 6.2 checks in the complete type source, generated structural CRD and
deepcopy output, and a full sample manifest. Kubernetes 1.36 envtest proves
schema installation, required fields, safe defaults, invalid combinations,
list uniqueness, immutable intent, one-way cancellation, printer columns, and
status-subresource isolation. Reconciliation and child-resource creation begin
in later sub-phases. Phase 6.8 implements the bounded status-attempt
progression described above.
