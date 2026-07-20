# AgentForge Codex Instructions

## Project

AgentForge is a Kubernetes-native platform for scheduling, executing, observing, building, and deploying isolated AI coding-agent workloads.

## Sources of truth

Before changing architecture or behavior, read in this order:

1. `AGENTS.md`
2. `specs/SPEC-MANIFEST.md`
3. `specs/PROJECT-STATUS.md`
4. `specs/TRACEABILITY.md`
5. The component specification governing the task
6. Relevant contracts, security requirements, testing requirements, and ADRs

Approved specifications are authoritative. Draft specifications may be refined, but contradictions must be surfaced rather than silently resolved.

## Required workflow

For every implementation task:

1. Inspect the repository and current diff before editing.
2. Identify the governing specifications and acceptance criteria.
3. Create a bounded implementation plan for non-trivial changes.
4. Implement the smallest complete vertical slice.
5. Add or update tests.
6. Run formatting, linting, focused tests, and relevant integration tests.
7. Review the diff for correctness, security, concurrency, and scope creep.
8. Update specifications, project status, and traceability when behavior changes.
9. Report changed files, commands run, test results, and unresolved risks.

## Architecture invariants

- PostgreSQL is the authoritative transactional store.
- Database state changes and integration events use a transactional outbox.
- Kafka consumers are idempotent and tolerate at-least-once delivery.
- Agent runs and deployments follow documented state machines.
- Kubernetes controllers reconcile desired and observed state idempotently.
- Untrusted agent workloads receive no platform or cloud administrator credentials.
- Production deployment state is managed through GitOps.
- Every tenant-owned query is tenant scoped.
- Redis is never the authoritative store for workflow, billing, audit, or approval state.
- Immutable image digests are used for deployments.

## Security prohibitions

Do not:

- add privileged containers without an approved ADR and threat-model update;
- mount the host Docker socket into workloads;
- use `hostPath` for agent execution;
- log secrets, raw authorization headers, unrestricted prompts, or source contents;
- trust tenant identifiers supplied directly by clients;
- weaken authorization, sandboxing, or policy checks to make tests pass;
- expose raw provider API keys to agent workloads;
- introduce cross-tenant database operations without explicit tests.

## Contract and migration policy

- Public API, event, CRD, database, and GitOps contract changes require specification and test updates.
- Breaking changes require versioning and migration or compatibility documentation.
- Database changes use forward migrations and documented rollback limitations.
- Generated artifacts must be reproducible from checked-in source and commands.

## Validation commands

As the repository is implemented, maintain these stable entry points:

```bash
make check-tools
make format
make lint
make test
make test-integration
make test-controller
make verify
```

A command must fail clearly when prerequisites are unavailable. It must never report false success.

## Change discipline

Do not:

- create a new service without an approved architectural reason;
- add dependencies without documenting purpose and operational impact;
- modify unrelated files;
- leave placeholders presented as completed functionality;
- invent scale, availability, security, or test claims;
- mark requirements implemented without code and test evidence.

## Completion response

Every completed task report must include:

- Summary
- Files changed
- Commands run
- Test results
- Specification and traceability updates
- Remaining risks or follow-up work
