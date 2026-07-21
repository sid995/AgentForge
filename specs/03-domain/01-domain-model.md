# Domain Model

## Aggregates

### Project

Owns runtime defaults, repository configuration, environments, policy bindings, and budgets.

### AgentRun

Owns user intent, lifecycle state, retry constraints, selected cluster,
idempotency result, and references to attempts and artifacts. An accepted run
begins in `QUEUED` with a pending first attempt; prompts are represented by a
secure reference rather than arbitrary text in logs or lifecycle events.

### AgentRunAttempt

Owns one monotonic execution attempt for a run. Attempts are numbered from one
per run and preserve their terminal outcome when a run is manually retried.

### Build

Owns source artifact reference, builder version, image digest, SBOM, scan result, and provenance.

### Deployment

Owns target environment, release digest, approval, Git revision, Argo application, and rollout state.

### Budget

Owns limits, period, reservations, and enforcement mode.

## Value objects

- TenantID, ProjectID, RunID, AttemptID.
- ResourceRequest.
- RuntimeDefinition.
- ArtifactReference.
- ImageDigest.
- Money and UsageQuantity.
- PolicyDecision.

## Invariants

- A terminal run cannot return to an active state except through the documented
  authorized manual-retry transition for an explicitly retryable failure; that
  transition creates a fresh monotonic attempt and preserves prior history.
- Attempts are numbered monotonically per run.
- Every mutable run and attempt transition compares and increments its version.
- An idempotency key is unique per tenant and must bind to one effective create
  request and result.
- Deployment references an accepted immutable build digest.
- Tenant IDs on related records must match.
- Cost ledger entries are append-only.
