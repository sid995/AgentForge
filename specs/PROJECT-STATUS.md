# AgentForge Project Status

**Last updated:** 2026-07-21

## Current milestone

Phase 3.2: AgentRun and AgentRunAttempt persistence

## Overall state

Phase 2 is complete. Phase 3.2 now adds durable AgentRun and AgentRunAttempt
persistence: validated state/failure value objects, repeatable migrations,
tenant-scoped idempotency uniqueness, transaction-local RLS, and
optimistic-locking repository writes. Tenant-owned HTTP endpoints remain
deferred until the authenticated Phase 3.3 API vertical slice.

## Completed

- Product and scope specifications
- High-level architecture
- Domain model and state-machine specifications
- Service specifications
- REST and event contract specifications
- Data, Kubernetes, security, observability, testing, delivery, and operations specifications
- LLM-assisted development workflow
- Complete GPT-5.6 Codex phase-by-phase execution playbook
- Root Codex instructions
- Bootstrap audit and execution-playbook path alignment
- Specification manifest ownership, status, implementation-path, and test-path metadata
- Development harness, repository verification, and CI workflow skeleton
- Phase 0 completion audit
- Phase 1 Platform API foundation: Go workspace, health endpoints, bounded
  configuration, JSON logging, HTTP middleware, graceful shutdown, unit tests,
  a non-root container image, and linker-injected build metadata
- Phase 2 PostgreSQL persistence: bounded `database/sql` pool using the `pgx`
  driver, migration command and tracking, local Compose integration tests,
  Tenant and Project repositories, transaction-local tenant context, project
  RLS, and migration/application database roles

## In progress

- Phase 3.2 AgentRun and AgentRunAttempt persistence

## Not started

- Kafka and outbox
- Scheduler
- Kubernetes Operator
- Agent Runner
- Model Gateway
- Build and deployment services
- Cloud infrastructure

## Known gaps to resolve during bootstrap

- Create ADRs for decisions currently described only inside component specifications.
- Add executable OpenAPI, event schema, CRD, and Terraform artifacts during their implementation phases.

## Next tasks

1. Commit the validated Phase 3.2 AgentRun and AgentRunAttempt persistence.
2. Implement Phase 3.3 authenticated create/get/list run API vertical slice.
