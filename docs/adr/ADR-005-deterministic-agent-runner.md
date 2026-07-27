# ADR-005: Deterministic Agent Runner and manifest-last artifacts

- Status: Accepted
- Date: 2026-07-27
- Owners: Agent Runner, Platform engineering, Security

## Context

The AgentRun Operator creates an isolated Job but must not interpret arbitrary
workload output or receive task, provider, or storage credentials. The platform
needs a deterministic first executable to prove the workload boundary before
introducing real model or source integrations. A successful Job is meaningful
only when its required artifacts are durably available.

## Decision

Implement the runner as a separate Go module and non-root container image. It
accepts only an Operator-mounted runtime configuration and a signed task
envelope from an already projected Secret. It verifies an Ed25519 detached
signature against a public trust bundle from a non-secret configuration mount.

The runner initially generates only controlled templates and executes explicit
argv commands through a confined executor. It has no shell interpolation,
Kubernetes API token, provider credential, arbitrary source retrieval, or
model client.

The runner exposes a small write-oriented artifact boundary: `Put` and `Head`
for workspace-local and S3-compatible storage. It publishes mandatory objects
under deterministic tenant/project/run/attempt keys and writes the final result
manifest last. Only after manifest verification does it write the existing
versioned termination evidence consumed by the Operator.

## Alternatives considered

- Put arbitrary task content in the AgentRun CR or ConfigMap: rejected because
  those objects are observable control-plane state and are not a secret store.
- Let the Operator collect logs or upload artifacts: rejected because it would
  broaden controller credentials and couple Kubernetes reconciliation to
  workload data handling.
- Treat zero process exit as success: rejected because a Pod can exit before
  artifacts are durable.
- Implement the full object-storage API now: rejected because metadata,
  downloads, presigning, deletion, and cross-tenant access belong to Phase 8.

## Consequences

The Runner has its own Go module, image, test targets, and the MinIO S3 client
dependency. Phase 7 adds no database migration, REST endpoint, event schema,
or CRD field. Signed tasks require a pre-provisioned projected Secret and
public trust configuration; brokered Vault/source retrieval remains deferred.

## Validation

Unit and adversarial tests cover signature verification, identity checks,
confinement, cancellation, redaction, and artifact ordering. Container and
MinIO tests prove durable artifacts. The Operator kind scenario is the intended
real-Job/PVC evidence gate, but remains unverified until the pinned kind node
image is available; status and traceability retain that pending state.

## Revisit conditions

Revisit when a task broker, source service, model gateway, or general artifact
service becomes available, or when production object storage requires a
different credential or upload protocol.
