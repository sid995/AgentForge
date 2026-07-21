# AgentForge

AgentForge is a Kubernetes-native infrastructure platform for running autonomous coding-agent workloads in isolated environments, tracking their execution and cost, building validated container artifacts, and deploying generated applications through GitOps.

This repository contains the complete specification and GPT-5.6 Codex execution
playbook, plus the Phase 1 Platform API foundation. Production implementation
proceeds phase by phase rather than asking one coding agent to manifest a cloud
platform through optimism.

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
- [`.env.example`](.env.example): documented local configuration through Phase 3.2

## Current state

Phase 2 is complete, and Phase 3.2 adds the durable AgentRun/attempt domain:
PostgreSQL migrations, tenant-scoped persistence, explicit transition
validation, idempotency uniqueness, and optimistic concurrency controls.
Tenant-owned HTTP APIs remain deferred until the Phase 3.3 identity and API
vertical slice; see [`specs/PROJECT-STATUS.md`](specs/PROJECT-STATUS.md).
