# Domain Model

## Aggregates

### Project

Owns runtime defaults, repository configuration, environments, policy bindings, and budgets.

### AgentRun

Owns user intent, lifecycle state, retry constraints, selected cluster, and references to attempts and artifacts.

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

- A terminal run cannot return to an active state.
- Attempts are numbered monotonically per run.
- Deployment references an accepted immutable build digest.
- Tenant IDs on related records must match.
- Cost ledger entries are append-only.
