# AgentForge Project Status

**Last updated:** 2026-07-21

## Current milestone

Phase 4.4: Idempotent-consumer foundation

## Overall state

Phase 2 and the Phase 3.1/3.2 state-machine and persistence foundation are
complete. At the user's direction, work has moved to Phase 4 event architecture
before Phase 3.3 through 3.5 API, cancellation, and retry commands. Those gaps
remain explicit dependencies for their corresponding outbox event integrations.

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
- Phase 3.1 normalized AgentRun/attempt state machines and Phase 3.2 durable
  AgentRun/attempt persistence with optimistic concurrency and tenant RLS
- Phase 4.1 event architecture and transactional-outbox ADR
- Phase 4.2 transactional AgentRun/outbox insertion, isolated relay role,
  lease-based competing claims, durable publication retries, published-only
  retention cleanup, provider-independent relay loop, and failure-window tests
- Phase 4.3 strict event validation and first executable schema/fixture,
  idempotent Kafka producer adapter, manual-ack consumer-group base, pinned
  root-Compose Redpanda, reproducible topic bootstrap, and broker contract tests

## In progress

- Phase 4.4 idempotent-consumer transaction and retry/DLQ foundation

## Not started

- Phase 3.3 create/get/list run API
- Phase 3.4 cancellation command
- Phase 3.5 retry command and Phase 3 audit
- Phase 4.5 event compatibility validation and Phase 4 audit
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

1. Commit the validated Phase 4.3 Kafka adapter and contracts.
2. Implement Phase 4.4 processed-event storage, explicit consumer transaction
   boundaries, retry classification, and DLQ publication.
3. Preserve the unresolved cancellation/retry event wiring until their Phase
   3.4/3.5 command transactions exist.
