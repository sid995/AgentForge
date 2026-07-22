# Phase 6 Plan: Kubernetes AgentRun CRD and Operator

## Status and canonical sources

Phase 6 is approved on `phases/phase-6`. The supplied prompt uses obsolete
paths. The manifest's canonical sources are `04-services/04-agent-operator.md`,
`08-kubernetes/01-crd-agent-run.md`, `08-kubernetes/02-workload-resources.md`,
`08-kubernetes/03-controller-behavior.md`,
`09-security/03-sandbox-and-network.md`,
`11-infrastructure/01-local-environment.md`, and `12-testing/`.

Each numbered sub-phase is generated or implemented, validated, reviewed, and
committed before the next begins. Phase 6 does not add a phase-specific Compose
file. The root Compose stack remains for non-Kubernetes local dependencies;
envtest and the later pinned kind gate own isolated Kubernetes test state.

## Component and ownership boundary

ADR-004 establishes `execution.agentforge.dev/v1alpha1` `AgentRun` as the
namespaced desired-state contract. The Scheduler chooses a cluster and creates
or compares the deterministic CR in Phase 6.10. The Operator alone reconciles
execution resources. PostgreSQL remains authoritative for platform workflow
state; Kubernetes status records observed cluster state and is handed back via
explicit lifecycle facts.

The Operator is a separate Go module because it has a distinct Kubernetes
dependency graph, release artifact, RBAC identity, test environment, and
deployment lifecycle. It remains in this monorepo and root Go workspace.

## Pinned generation and test toolchain

- Kubebuilder CLI `v4.14.0` generated the project and API skeleton.
- controller-runtime `v0.24.1` with Kubernetes libraries `v0.36.1` matches the
  repository's Go `1.26` baseline.
- controller-gen `v0.20.1`, kustomize `v5.8.1`, setup-envtest `v0.24.1`,
  Kubernetes envtest assets `1.36.0`, and golangci-lint `v2.11.4` are pinned in
  `operator/Makefile`.
- `make operator-manifests operator-generate` regenerates checked-in CRD/RBAC
  and deepcopy output. A second clean generation must produce no diff.

## Sub-phase boundaries

### 6.1 Bootstrap

Create the Operator module, API group/version and empty owned schema skeleton,
scheme registration, manager startup, leader-election configuration,
authenticated metrics, health/readiness probes, JSON structured logging,
envtest foundation, nested instructions, root Make/CI integration, and a
non-root image. No reconciler or execution resource creation exists yet.

### 6.2 CRD contract

Define every approved desired-state/status field, structural schema validation,
safe defaults, cross-field CEL rules, printer columns, status subresource,
samples, schema tests, and future conversion strategy notes. This sub-phase
changes only the API contract and generated artifacts.

Status: implemented and validated against Kubernetes 1.36 envtest. The
sub-phase includes immutable Scheduler intent, one-way cancellation, bounded
attempt history, a complete sample, and generated structural schema/deepcopy
artifacts. No reconciler or child-resource creation is present.

### 6.3 Reconciliation foundation

Add fetch/not-found handling, generation validation, status initialization,
conditions, observed generation, finalizer policy, deterministic naming,
bounded error classification/backoff, metrics/log context, idempotency, and
status-conflict behavior without creating child resources.

### 6.4 Resource builders

Add pure builders and golden/security/mutation tests for ServiceAccount,
ConfigMap, secret references, PVC, NetworkPolicy, and Job. Builders enforce the
sandbox contract and perform no API writes.

### 6.5 Prerequisite reconciliation

Ensure resources in the specified order with server-side apply or an approved
ownership model, safe comparison/conflict behavior, storage-pending conditions,
and envtest coverage.

### 6.6 Job lifecycle

Derive status and failure classification from observed Job/Pod state, preserve
terminal stability and mandatory result evidence, and prove duplicate-safe
creation with envtest and a pinned kind integration gate.

### 6.7 Cancellation

Stop new work, gracefully terminate active work within a bounded deadline,
preserve diagnostics/partial-state opportunities, and make every cancellation
position and restart path idempotent.

### 6.8 Retry

Apply the approved retry taxonomy and attempt ceiling, use deterministic new
Job names and bounded jittered backoff, retain prior attempt metadata, and
never retry security, invalid-configuration, or unapproved test failures.

### 6.9 Finalization and cleanup

Use ownership for cluster-local garbage collection and a finalizer only for
external or retained resources. Cleanup is bounded, observable, idempotent,
restart-safe, and cannot silently leave a permanently blocked finalizer.

### 6.10 Scheduler handoff and audit

Consume `agent-run.scheduled.v1` idempotently, select the registered cluster
client, validate and create/compare the deterministic namespace/AgentRun,
handle conflict/retry/failure and tracing/audit context, and never create Jobs.
Then perform the security/controller review and the Phase 6 completion audit.
