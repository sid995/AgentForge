# Agent Operator Codex Instructions

## Scope

This module owns the `execution.agentforge.dev/v1alpha1` API, generated CRDs,
the controller manager, reconciliation code, and Kubernetes resource builders.

## Generated artifacts

- Never edit `config/crd/bases/*.yaml`, `config/rbac/role.yaml`, or
  `api/v1alpha1/zz_generated.deepcopy.go` directly.
- Change Go markers or source inputs, then run `make manifests generate` from
  this directory.
- Tool and envtest versions are pinned in this module's `Makefile`; do not use
  an unpinned global generator for committed output.
- A clean regeneration must produce no diff.

## Controller invariants

- Reconciliation is idempotent and derives status from observed Kubernetes
  state rather than in-memory progress.
- Names, labels, annotations, ownership, conditions, and retry decisions are
  deterministic and bounded.
- Avoid status-only reconciliation loops and unnecessary writes.
- Use owner references for cluster-local resources and finalizers only for
  external or explicitly retained resources.
- The Scheduler creates or updates `AgentRun` resources; only this Operator may
  create execution workloads.

## Security

- Never create privileged containers, host namespace access, `hostPath`,
  device mounts, or Docker socket mounts.
- Untrusted workloads run as non-root, drop all capabilities, disallow
  privilege escalation, use `RuntimeDefault` seccomp, and do not automount a
  Kubernetes API token unless an approved profile explicitly requires it.
- Logs and metrics must not expose secrets, prompts, source, credentials, raw
  authorization headers, or unbounded user-controlled values.

## Validation

Run `make manifests generate`, `make fmt vet`, `make test`, `make lint`, and
`make docker-build` for substantive Operator changes. Later lifecycle phases
also require the pinned kind integration gate.
