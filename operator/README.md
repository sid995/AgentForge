# AgentForge Agent Operator

The Agent Operator translates `execution.agentforge.dev/v1alpha1` `AgentRun`
desired state into reconciled Kubernetes resources. Phase 6.1 provides only
the API/CRD and controller-manager foundation; it deliberately creates no
execution resources.

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

The Phase 6.1 sample contains only the empty bootstrap schema. Phase 6.2 owns
the complete desired-state and status contract.
