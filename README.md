# AgentForge

AgentForge is a Kubernetes-native infrastructure platform for running autonomous coding-agent workloads in isolated environments, tracking their execution and cost, building validated container artifacts, and deploying generated applications through GitOps.

This repository contains the complete specification and GPT-5.6 Codex execution
playbook plus the implemented control-plane foundations through Phase 6.6.
Production implementation proceeds through validated, committed sub-phases.

## Start here

1. Read [`START-HERE.md`](START-HERE.md).
2. Open the repository in Codex.
3. Run the first five prompts in [`specs/15-llm-development/05-codex-execution-playbook.md`](specs/15-llm-development/05-codex-execution-playbook.md).
4. Keep [`specs/PROJECT-STATUS.md`](specs/PROJECT-STATUS.md) and [`specs/TRACEABILITY.md`](specs/TRACEABILITY.md) accurate after every task.

## Important documents

- [`AGENTS.md`](AGENTS.md): repository-wide Codex instructions
- [`specs/README.md`](specs/README.md): specification overview
- [`specs/SPEC-MANIFEST.md`](specs/SPEC-MANIFEST.md): complete specification index
- [`specs/PROJECT-STATUS.md`](specs/PROJECT-STATUS.md): current implementation status
- [`specs/TRACEABILITY.md`](specs/TRACEABILITY.md): requirement-to-code-and-test mapping
- [`specs/15-llm-development/05-codex-execution-playbook.md`](specs/15-llm-development/05-codex-execution-playbook.md): complete phased prompt sequence
- [`.env.example`](.env.example): documented local configuration through Phase 5

## Current state

Phase 2 and the Phase 3.1/3.2 AgentRun state/persistence foundation are
complete. Phase 4.1 now defines the transactional-outbox, at-least-once Kafka,
idempotent-consumer, retry/DLQ, replay, retention, security, and compatibility
architecture. Phase 3.3 through 3.5 command/API work remains explicitly
unfinished. Phase 4.2 adds the PostgreSQL transactional outbox and
provider-independent relay foundation. Phase 4.3 adds executable event
contracts, Kafka adapters, and pinned local Redpanda in the same root Compose
file. Phase 4.4 adds database-idempotent consumer transactions and retry/DLQ
routing. Phase 4.5 adds machine-readable schemas for every implemented event,
immutable-major compatibility baselines, producer contracts, supported
consumer fixtures, and CI validation. Phase 5.1 defines the Scheduler design;
Phase 5.2 adds fair competing PostgreSQL queue claims and expiring leases.
Phase 5.3 adds explained tenant/user/resource/budget eligibility decisions.
Phase 5.4 adds the validated, freshness-aware execution-cluster registry.
Phase 5.5 adds deterministic least-loaded and region-affinity selection; see
Phase 5.6 adds transactional capacity and budget reservations; see
Phase 5.7 atomically commits assignments, reservations, state, and outbox
events. Phase 5.8 adds the bounded Scheduler process, optional Kafka wake
hints, authoritative polling, health/metrics, and the root-Compose image; see
Phase 6.1 adds the Kubebuilder/controller-runtime Operator module and manager
foundation. Phase 6.2 adds the complete validated
`execution.agentforge.dev/v1alpha1` AgentRun desired/status contract, generated
CRD, full sample, and Kubernetes 1.36 schema tests. Phase 6.3 adds the
idempotent reconciliation/status/finalizer foundation without creating child
resources. Phase 6.4 adds pure secure workload-resource builders, and Phase
6.5 reconciles them in dependency order with safe ownership and storage gates.
Phase 6.6 adds observed Job/Pod lifecycle, strict result evidence, and a pinned
Kubernetes 1.36 kind gate; see
[`specs/PROJECT-STATUS.md`](specs/PROJECT-STATUS.md).

## Agent Operator development

The Operator has an independently pinned Kubernetes toolchain. The root entry
points delegate to its module:

```bash
make operator-manifests operator-generate
make test-controller
make test-controller-kind
make lint-controller
make build-operator
```

Generation downloads version-pinned tools into ignored `operator/bin/` paths.
Phase 6.6 also handles `WaitForFirstConsumer` binding and observed lifecycle.

## Local Scheduler

```bash
cp .env.example .env
docker compose up --detach postgres redpanda
make migrate
docker compose up --detach scheduler
curl --fail http://127.0.0.1:18081/health/ready
curl --fail http://127.0.0.1:18081/metrics
```

`make migrate` sources and exports `.env` in its recipe when the file exists;
otherwise it uses the caller's exported environment. `docker compose`
reads `.env` for interpolation and explicitly supplies the Scheduler's isolated
database URL; it does not export `.env` into the caller's shell. Run
`clusterctl` with an explicitly exported Scheduler database URL before testing
real scheduling; tenant policies must likewise exist for admitted tenants.

The root Compose stack caps processor and memory usage for every service. The
local defaults are PostgreSQL at `1.0` CPU and `512m`, Redpanda at `1.0` CPU
and `1g`, and Scheduler at `0.5` CPU and `256m`. Override the corresponding
`*_CPUS` and `*_MEMORY_LIMIT` values in `.env` when the development workload
needs a different budget.
