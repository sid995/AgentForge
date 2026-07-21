# AgentForge

AgentForge is a Kubernetes-native infrastructure platform for running autonomous coding-agent workloads in isolated environments, tracking their execution and cost, building validated container artifacts, and deploying generated applications through GitOps.

This repository contains the complete specification and GPT-5.6 Codex execution playbook, plus the Phase 1 Platform API foundation. Production implementation proceeds phase by phase rather than asking one coding agent to manifest a cloud platform through optimism.

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
- [`.env.example`](.env.example): documented local configuration through Phase 4.5

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
consumer fixtures, and CI validation; see
[`specs/PROJECT-STATUS.md`](specs/PROJECT-STATUS.md).
