# AgentForge Project Status

**Last updated:** 2026-07-23

## Current milestone

Phase 6.10 Scheduler-to-CR handoff complete; Phase 6 review and audit are next

## Overall state

Phase 2, the Phase 3.1/3.2 state-machine and persistence foundation, and Phase
4 and 5 are complete. Phase 6 implementation is complete pending its specialist
review and completion audit.
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
- Phase 6.2 complete AgentRun desired/status contract, immutable Scheduler
  intent and one-way cancellation, structural validation and safe defaults,
  printer columns, status subresource, generated artifacts, full sample, future
  conversion strategy, and Kubernetes 1.36 envtest schema coverage
- Phase 6.3 registered AgentRun reconciliation foundation with defensive
  validation, observed-generation status and conditions, retained-resource-only
  finalizer policy, deterministic names, transient/permanent errors, bounded
  exponential retry, correlated logs, low-cardinality metrics, conflict-safe
  minimal status writes, self-update filtering, and envtest coverage
- Phase 6.4 pure deterministic ServiceAccount, immutable ConfigMap, PVC,
  NetworkPolicy, and Job builders with trusted placement profiles, optional
  runtime class, secret/config projections, retained-storage ownership,
  restricted egress validation, full restricted Pod/container security,
  golden manifests, invalid-configuration tests, and security mutations
- Phase 6.5 ordered prerequisite reconciliation with non-forced server-side
  apply, explicit owner validation, matching-object no-ops, external metadata
  preservation, reference and storage gating, per-prerequisite conditions,
  owned-resource watches, conflict diagnostics, and Kubernetes 1.36 envtest
- Phase 6.6 observed Job/Pod lifecycle projection, normalized retry/permanent
  failure taxonomy, strict mandatory result evidence, current and per-attempt
  timestamps/status, terminal stability, deleted-Job no-recreation, first
  consumer PVC binding, exhaustive unit/envtest coverage, and digest-pinned
  kind 0.32.0/Kubernetes 1.36.1 integration
- Phase 6.7 cancellation-before-creation, graceful foreground Job termination,
  durable two-minute restart-safe deadline, exact owner-UID forced Pod
  termination, graceful/forced terminal status, completion-race preservation,
  repeated cancellation idempotency, and focused controller tests
- Phase 6.8 desired/platform retry-policy intersection, strict non-retryable
  security/configuration/test outcomes, attempt ceilings, deterministic capped
  exponential jitter, restart-safe deadlines, retained attempt history,
  status-before-resource ordering, and fresh deterministic retry Jobs
- Phase 6.9 owner-reference garbage collection plus retained-workspace-only
  finalization, every-attempt retention handoff, no-delete PVC preservation,
  partial/missing-resource idempotency, bounded cleanup diagnostics/retry,
  restart-safe ten-minute escalation, envtest deletion, and real kind PVC
  survival after AgentRun finalization
- Phase 6.10 separately deployable scheduled-intent handoff with a
  transactionally paired immutable intent record and unchanged v1 event
  contract, explicit registered-cluster client selection and tenant
  authorization, deterministic Namespace/AgentRun
  creation, exact idempotent comparison, durable processed markers,
  retry/DLQ behavior, status-subresource ownership preservation, correlated
  audit context, least-privilege database role, and bounded root-Compose
  packaging

## In progress

- Phase 6 controller/security review and completion audit

## Not started

- Phase 3.3 create/get/list run API
- Phase 3.4 cancellation command
- Phase 3.5 retry command and Phase 3 audit
- Agent Runner
- Model Gateway
- Build and deployment services
- Cloud infrastructure

## Known gaps to resolve during bootstrap

- Create ADRs for decisions currently described only inside component specifications.
- Add executable OpenAPI, event schema, CRD, and Terraform artifacts during their implementation phases.

## Next tasks

1. Perform the Phase 6 Kubernetes-controller and security review.
2. Run the Phase 6 completion audit and full repository gates.
3. Preserve the unresolved Phase 3 cancellation/retry command dependencies.
