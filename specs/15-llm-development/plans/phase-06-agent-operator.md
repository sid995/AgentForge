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

Status: implemented and validated. The controller uses generation/deletion
predicates, semantic status comparison, optimistic status patches, bounded
error classification and rate limiting, deterministic names, low-cardinality
metrics, correlated logs, and a finalizer only for retained workspaces.

### 6.4 Resource builders

Add pure builders and golden/security/mutation tests for ServiceAccount,
ConfigMap, secret references, PVC, NetworkPolicy, and Job. Builders enforce the
sandbox contract and perform no API writes.

Status: implemented and validated. Pure builders produce deterministic owned
resources, enforce trusted scheduling profiles and restricted egress, preserve
retained PVCs, project configuration and secrets read-only, and apply the full
Pod/container security contract. Golden, invalid-configuration, security, and
mutation tests are checked in.

### 6.5 Prerequisite reconciliation

Ensure resources in the specified order with server-side apply or an approved
ownership model, safe comparison/conflict behavior, storage-pending conditions,
and envtest coverage.

Status: implemented and validated. Reconciliation now uses non-forced
server-side apply with a stable field owner, verifies existing ownership,
preserves external metadata, stops on missing references or pending storage,
and creates NetworkPolicy and Job only after the PVC is bound. Kubernetes 1.36
envtest covers order, idempotency, conditions, conflicts, and storage gating.

### 6.6 Job lifecycle

Derive status and failure classification from observed Job/Pod state, preserve
terminal stability and mandatory result evidence, and prove duplicate-safe
creation with envtest and a pinned kind integration gate.

Status: implemented and validated. Observed Job/Pod state now projects pending,
scheduled, container-creating, running, success, and normalized failure states
plus bounded attempt history and timestamps. Success requires strict versioned
termination evidence. Terminal attempts are stable, deleted observed Jobs are
not recreated, and `WaitForFirstConsumer` storage safely triggers binding.
Unit, Kubernetes 1.36 envtest, and digest-pinned kind 0.32/Kubernetes 1.36.1
integration gates cover the lifecycle.

### 6.7 Cancellation

Stop new work, gracefully terminate active work within a bounded deadline,
preserve diagnostics/partial-state opportunities, and make every cancellation
position and restart path idempotent.

Status: implemented and validated. Cancellation precedes resource creation,
uses foreground Job deletion for the runner grace period, persists a two-minute
deadline in status across restarts, and applies zero-grace deletion only to
exact owner-UID Pods after expiry. Graceful and forced terminal outcomes,
completion races, repetition, and already-missing Jobs are covered.

### 6.8 Retry

Apply the approved retry taxonomy and attempt ceiling, use deterministic new
Job names and bounded jittered backoff, retain prior attempt metadata, and
never retry security, invalid-configuration, or unapproved test failures.

Status: implemented and validated. Desired retry categories are intersected
with the platform's transient/internal allowlist and attempt ceiling. Backoff
is exponential, capped, deterministically jittered, and restart-stable. Status
commits the new attempt before fresh attempt-qualified resources are created,
while prior attempt and failed Job history remain intact.
Kubernetes 1.36.1 kind also proves a transient pull failure creates distinct
attempt-one and attempt-two Jobs.

### 6.9 Finalization and cleanup

Use ownership for cluster-local garbage collection and a finalizer only for
external or retained resources. Cleanup is bounded, observable, idempotent,
restart-safe, and cannot silently leave a permanently blocked finalizer.

Status: implemented and validated. Disposable children remain owner-managed;
the retained-workspace finalizer validates and marks every attempt PVC as
released without deleting it. Missing/partial resources, API failure, restart,
and ownership conflicts are idempotent and diagnosable. A deletion-timestamp
anchored ten-minute deadline releases the finalizer with an explicit runbook
escalation path, and kind proves the retained PVC survives CR deletion.

### 6.10 Scheduler handoff and audit

Consume `agent-run.scheduled.v1` idempotently, select the registered cluster
client, validate and create/compare the deterministic namespace/AgentRun,
handle conflict/retry/failure and tracing/audit context, and never create Jobs.
Then perform the security/controller review and the Phase 6 completion audit.

Status: implemented and validated. A separately deployable handoff consumer
preserves the Scheduler process's no-Kubernetes boundary. Atomic scheduled
facts are transactionally paired with complete immutable CR intent keyed by
event ID while the published v1 contract remains unchanged. The consumer uses
durable duplicate markers, explicit cluster-context selection and tenant authorization,
deterministic names, exact existing-resource comparison, retry/DLQ routing,
and correlated audit annotations. It creates only Namespace and AgentRun
objects and leaves status exclusively to the Operator.

The handoff package reuses the checked-in Operator API type and adds
Kubernetes `client-go`/controller-runtime client dependencies to the Platform
API Go module. This increases module download and source-build cost for that
module; only the handoff binary initializes Kubernetes clients, while Platform
API and Scheduler runtime behavior remains unchanged.
