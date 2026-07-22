# AgentForge Agent Operator

The Agent Operator translates `execution.agentforge.dev/v1alpha1` `AgentRun`
desired state into reconciled Kubernetes resources. Phase 6.1 provides the
controller-manager foundation. Phase 6.2 adds the complete validated CRD
contract, but deliberately creates no execution resources.

## Pinned toolchain

- Go 1.26
- controller-runtime 0.24.1 / Kubernetes libraries 0.36.1
- controller-gen 0.20.1
- kustomize 5.8.1
- setup-envtest 0.24.1 with Kubernetes 1.36.0 assets
- golangci-lint 2.11.4

All downloaded tools are installed under ignored `operator/bin/` paths.

## Generate and verify

Run from `operator/`:

```bash
make manifests generate
git diff --exit-code -- api/v1alpha1/zz_generated.deepcopy.go config/crd/bases config/rbac/role.yaml
make test
make lint
make docker-build IMG=agentforge/operator:dev
```

`make manifests` regenerates CRDs and RBAC from source markers. `make generate`
regenerates Go deepcopy implementations. The pinned year and generator
versions make checked-in artifacts reproducible.

## Manager endpoints and deployment

The deployment enables leader election, exposes `/healthz` and `/readyz` on
port `8081`, and serves authenticated HTTPS metrics on port `8443`. The manager
uses JSON structured logging by default and runs as a non-root distroless
container with a read-only root filesystem and dropped capabilities.

```bash
make install
make deploy IMG=<registry>/agentforge-operator:<immutable-tag>
```

The checked-in sample demonstrates the complete Phase 6.2 desired-state
contract. Kubernetes 1.36 envtest verifies defaults, validation and prohibited
combinations, immutable execution intent, one-way cancellation, printer
columns, and status-subresource isolation.

Phase 6.3 registers the reconciliation foundation. It initializes observed
status and conditions, uses deterministic names and bounded retry, suppresses
self-update loops, and adds a finalizer only for retained workspaces. Phase 6.4
owns pure secure resource builders.

Phase 6.4 adds pure builders for the tokenless ServiceAccount, immutable
configuration, retained-or-owned workspace, deny-by-default network policy,
and restricted Job. Trusted profiles provide dedicated-node placement,
tolerations, topology spread, priority, and optional runtime class. Golden and
security mutation tests lock the manifests.

Phase 6.5 creates prerequisites in the strict ServiceAccount, references and
configuration, PVC, NetworkPolicy, Job order. It uses non-forced server-side
apply, rejects conflicting ownership, preserves unrelated metadata, requeues
missing references and pending storage, and exposes one condition per gate.
Envtest proves matching-resource no-ops and that ordinary pending storage blocks
network and compute creation. `WaitForFirstConsumer` storage is detected and
permits the Job consumer required to trigger binding.

Phase 6.6 projects observed Job and Pod lifecycle into current and per-attempt
status, including scheduling, active execution, timestamps, normalized failure
taxonomy, and stable terminal state. A completed Job succeeds only when its
runner termination message contains strict versioned artifact-manifest
evidence; raw messages are never projected. An externally deleted observed Job
fails without recreation.

Phase 6.7 reconciles one-way cancellation before any ordinary prerequisite
work. Runs without a Job cancel immediately. Active Jobs receive a foreground
delete so the Pod's configured grace period can be used for partial-state
upload; a durable condition anchors a two-minute maximum deadline across
controller restarts. After that deadline, only Pods whose controller UID
matches the observed Job are force-deleted. Status distinguishes graceful and
forced cancellation, retains existing diagnostics, and remains idempotent.

Run the real lifecycle gate with:

```bash
make test-kind
```

The target downloads kind 0.32.0 into `bin/` and creates a disposable two-node
Kubernetes 1.36.1 cluster from a digest-pinned image. It proves PVC consumer
binding, real Pod completion, mandatory-evidence enforcement, attempt status,
and duplicate-safe Job creation.
