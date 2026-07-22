# AgentForge Project Status

**Last updated:** 2026-07-22

## Current milestone

Phase 6.1 Operator bootstrap complete; Phase 6.2 CRD contract is next

## Overall state

Phase 2, the Phase 3.1/3.2 state-machine and persistence foundation, and Phase
4 and 5 are complete. Phase 6 Operator work has begun after the Phase 5 merge.
At the user's direction, Phase 5 Scheduler was completed
before Phase 3.3 through 3.5 API, cancellation, and retry commands. Those gaps
remain explicit dependencies for their corresponding command integrations.

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
- Phase 4.4 processed-event marker/business transactions, database-enforced
  duplicate no-ops across replicas and acknowledgement crashes, explicit
  transient/permanent failures, retry topics, sanitized DLQ envelopes, and
  malformed-message fingerprint quarantine
- Phase 4.5 machine-readable schemas for all implemented events, immutable-
  major compatibility baselines, producer contracts, supported v1 consumer
  fixtures, CI/Make validation, distributed-systems review, and completion audit
- Phase 5.1 Scheduler implementation plan and ADR-003 covering competing
  PostgreSQL claims, fairness, quotas, registry selection, reservations,
  process lifecycle, observability, and failure/concurrency testing
- Phase 5.2 bounded fair queue claims, `FOR UPDATE SKIP LOCKED`, expiring and
  renewable Scheduler leases, least-privilege cross-tenant role, queue-age
  telemetry hooks, safe structured logs, and concurrency/isolation tests
- Phase 5.3 explained eligibility policies for tenant/project state, supported
  runtime/profile, tenant/user concurrency, queue, CPU, memory, and daily
  budget; indexed locked evaluation, persisted decisions, and noisy-neighbour
  concurrency tests
- Phase 5.4 validated global execution-cluster metadata, append-only capacity
  snapshots, tenant allowlists, optimistic updates, freshness-aware candidate
  filtering, an internal registration CLI, and PostgreSQL integration tests
- Phase 5.5 replaceable least-loaded and region-affinity strategies with
  integer scoring, deterministic tie-breaking, temporary no-capacity deferral,
  bounded configuration, safe decision logging, and fallback tests
- Phase 5.6 transactional, attempt-attached capacity and budget reservations
  with locked admission, active-attempt uniqueness, exact-replay idempotency,
  release/settlement, expired reclaim, and overbooking concurrency tests
- Phase 5.7 atomic lease/policy/cluster revalidation, capacity and budget
  reservation, attempt assignment, `PROVISIONING` transition, versioned
  scheduled/capacity-wait outbox contracts, exact replay, and concurrency tests
- Phase 5.8 independently runnable Scheduler with bounded workers/backpressure,
  authoritative polling, optional Kafka wake hints, bounded jitter/backoff,
  graceful shutdown, PostgreSQL readiness, liveness/metrics, non-root image,
  root Compose integration, service tests, and Phase 5 completion audit
- Phase 6.1 Kubebuilder/controller-runtime Operator module, namespaced
  `execution.agentforge.dev/v1alpha1` API and generated CRD/deepcopy foundation,
  scheme registration, leader-election deployment, health/readiness,
  authenticated metrics, structured logging, pinned generation/envtest tools,
  root Make/CI integration, and non-root image

## In progress

- Phase 6.2 complete AgentRun CRD desired-state and status contract

## Not started

- Phase 3.3 create/get/list run API
- Phase 3.4 cancellation command
- Phase 3.5 retry command and Phase 3 audit
- Phase 6.2 through 6.10 Operator contract, reconciliation, workload lifecycle,
  cancellation, retry, cleanup, and Scheduler handoff
- Agent Runner
- Model Gateway
- Build and deployment services
- Cloud infrastructure

## Known gaps to resolve during bootstrap

- Create ADRs for decisions currently described only inside component specifications.
- Add executable OpenAPI, event schema, CRD, and Terraform artifacts during their implementation phases.

## Next tasks

1. Implement and commit Phase 6.2 AgentRun CRD contract and schema tests.
2. Preserve the unresolved Phase 3 cancellation/retry command dependencies and
   the Phase 6 no-Kubernetes boundary.
