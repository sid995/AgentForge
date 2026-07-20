# AgentForge: GPT-5.6 Codex Execution Playbook

This playbook is the ordered prompt sequence for building AgentForge with GPT-5.6 in Codex.

Use one Codex thread per bounded task unless a prompt explicitly says otherwise. Do not ask Codex to build an entire phase in one unreviewed pass. Each implementation task should end with tests, specification updates, and a concise handoff.

---

# 0. Operating rules

## 0.1 Standard completion contract

Append this block to implementation prompts unless the prompt already contains a stricter completion section:

```text
Completion requirements:

1. Inspect the existing repository before editing.
2. Follow every applicable AGENTS.md file.
3. Implement only the requested scope.
4. Add or update tests.
5. Run formatting, linting, unit tests, and relevant integration tests.
6. Update specifications, traceability, and project status when behaviour changes.
7. Do not hide failing tests or weaken checks to obtain a passing result.
8. Report:
   - summary
   - files changed
   - commands run
   - test results
   - specification updates
   - unresolved risks
```

## 0.2 Standard review prompt

Use after every substantial implementation task:

```text
Review the current uncommitted diff as a senior infrastructure engineer.

Read the governing specifications and applicable AGENTS.md files.

Check:

- correctness
- architectural consistency
- concurrency and race conditions
- idempotency
- failure recovery
- tenant isolation
- security boundaries
- observability
- migration safety
- test quality
- unnecessary complexity
- undocumented behaviour

Do not edit files initially.

Return findings ordered by severity, with exact file references and concrete fixes.
If there are no material findings, state that explicitly and list residual risks.
```

## 0.3 Standard fix prompt

Use after reviewing findings:

```text
Address the approved review findings from the previous review.

Constraints:

- Make the smallest safe changes.
- Do not refactor unrelated code.
- Add regression tests for every corrected defect where practical.
- Re-run all affected validation.
- Update specifications only when externally visible behaviour changed.

Report each finding and how it was resolved.
```

## 0.4 Standard phase-gate prompt

Use at the end of every phase:

```text
Perform the phase completion audit.

Read:

- AGENTS.md
- specs/SPEC-MANIFEST.md
- specs/PROJECT-STATUS.md
- specs/TRACEABILITY.md
- the current phase specification
- all changed code and tests from this phase

Verify:

1. Every phase acceptance criterion is either proven or marked incomplete.
2. Required tests exist and pass.
3. Specifications match implementation.
4. No placeholder is presented as complete.
5. Security controls were not weakened.
6. Public contracts are versioned and documented.
7. Migrations and rollback procedures are safe.
8. Operational metrics and logs exist for new workflows.
9. TRACEABILITY.md and PROJECT-STATUS.md are accurate.
10. Remaining debt is listed with severity and owner phase.

Write the audit to:
specs/12-llm-development/audits/<phase-id>-completion-audit.md

Do not mark the phase complete when critical findings remain.
```

---

# 1. Local machine bootstrap

Run these commands manually before opening the project in Codex.

```bash
mkdir agentforge
cd agentforge
git init
git branch -M main
mkdir -p specs
touch README.md AGENTS.md
```

Optional tools expected later:

```bash
git --version
go version
docker --version
kubectl version --client
helm version
kind version
terraform version
make --version
```

Do not install every tool before Phase 0. Codex should first document and verify the expected environment.

---

# 2. Phase 0: Repository and specification bootstrap

## Goal

Create a trustworthy repository skeleton, instruction hierarchy, specification index, development commands, and status tracking before production code exists.

## Prompt 0.1: Inspect supplied specification bundle

```text
You are bootstrapping the AgentForge repository.

Do not write production code.

Tasks:

1. Inspect every existing file in the repository.
2. Locate the supplied AgentForge specification bundle.
3. Create a concise inventory of:
   - available specifications
   - missing specifications
   - contradictory requirements
   - stale or broken file references
   - unclear architectural decisions
4. Propose the canonical repository layout.
5. Propose the root AGENTS.md responsibilities.
6. Propose the first implementation milestone dependency order.

Write the result to:
specs/12-llm-development/bootstrap-audit.md

Do not silently resolve architectural contradictions. Record them under
"Decisions required".
```

## Prompt 0.2: Create specification repository structure

```text
Create the AgentForge specification repository structure.

Required top-level specification areas:

- 00-product
- 01-architecture
- 02-domain
- 03-services
- 04-contracts
- 05-data
- 06-infrastructure
- 07-security
- 08-observability
- 09-testing
- 10-delivery
- 11-operations
- 12-llm-development
- 13-roadmap
- adr

Create or normalize:

- specs/README.md
- specs/SPEC-MANIFEST.md
- specs/PROJECT-STATUS.md
- specs/TRACEABILITY.md
- specs/GLOSSARY.md

Requirements:

- Preserve useful existing specification content.
- Do not create empty files merely to satisfy a directory list.
- Each manifest entry must include status, owner component, implementation path,
  and test path where applicable.
- Mark uncertain content as Draft.
- Mark accepted architectural constraints as Approved only when already explicit.
- Add stable requirement identifiers such as FR-RUN-001 and SEC-EXE-001.
- Add links between related specifications.

Do not create application code.
```

## Prompt 0.3: Create root AGENTS.md

```text
Create the root AGENTS.md for AgentForge.

It must define:

- project purpose
- sources of truth
- specification lookup order
- mandatory task workflow
- architecture invariants
- tenant-isolation requirements
- testing and validation expectations
- dependency policy
- contract-change policy
- migration policy
- security prohibitions
- documentation-update policy
- completion-report format

Keep it concise enough to remain useful in every Codex task.

Also create nested AGENTS.md files only for currently existing major boundaries.
Do not create dozens of speculative instruction files.
```

## Prompt 0.4: Add development harness skeleton

```text
Create the development harness skeleton without implementing product features.

Add:

- Makefile
- .editorconfig
- .gitignore
- .gitattributes
- .golangci.yml or documented lint configuration
- scripts/check-tools.sh
- scripts/verify-repository.sh
- docs for local prerequisites
- GitHub Actions workflow skeleton for formatting and repository checks

The Makefile should initially expose:

- help
- check-tools
- format
- lint
- test
- verify

Commands may report that implementation-specific checks are not yet available,
but they must fail clearly rather than pretending success.

Do not add PostgreSQL, Kafka, Redis, or Kubernetes yet.
```

## Prompt 0.5: Bootstrap audit and first commit preparation

```text
Audit Phase 0.

Verify:

- repository paths referenced by specifications exist
- all Markdown links are valid where practical
- AGENTS.md instructions are non-contradictory
- Make targets are documented
- PROJECT-STATUS.md reflects reality
- TRACEABILITY.md contains no false implementation claims
- no production feature is marked implemented

Fix documentation and harness defects found by the audit.

Then prepare a suggested commit message and commit body.
Do not commit unless explicitly instructed.
```

### Phase 0 acceptance

```text
- Codex can identify governing specs for any future task.
- make help works.
- make verify performs meaningful repository checks.
- PROJECT-STATUS.md says no product features are implemented.
- Specification links are navigable.
```

---

# 3. Phase 1: Go workspace and platform API foundation

## Goal

Create the first deployable service with health checks, configuration, logging, graceful shutdown, and tests.

## Prompt 1.1: Plan Phase 1

```text
Plan Phase 1: Platform API foundation.

Read:

- AGENTS.md
- specs/SPEC-MANIFEST.md
- specs/01-architecture/*
- specs/03-services/platform-api.md
- specs/08-observability/logging.md
- specs/09-testing/testing-strategy.md
- specs/13-roadmap/milestones.md

Do not edit code.

Produce:

- proposed package structure
- dependency choices with justification
- configuration model
- health endpoint behaviour
- graceful-shutdown behaviour
- test plan
- exact files to create
- acceptance criteria
- risks

Write the plan to:
specs/12-llm-development/plans/phase-01-platform-api-foundation.md
```

## Prompt 1.2: Initialize Go workspace

```text
Implement the Go workspace and Platform API service skeleton.

Scope:

- initialize go.work and the service module
- create services/platform-api
- create cmd entrypoint
- implement configuration loading from environment
- implement structured JSON logging
- implement process startup and graceful shutdown
- implement GET /health/live
- implement GET /health/ready
- expose build/version information
- add unit tests
- add service Dockerfile
- extend Makefile targets

Do not add:

- database
- Redis
- Kafka
- authentication
- project or run endpoints
- Kubernetes

Use dependency injection from the beginning.
Avoid framework-heavy abstractions unless justified in the phase plan.
```

## Prompt 1.3: Add API middleware foundation

```text
Add the Platform API middleware foundation.

Implement:

- request ID generation and propagation
- trace-context passthrough placeholder compatible with later OpenTelemetry
- panic recovery
- request logging
- request timeout
- response content type
- bounded request body size
- consistent JSON error envelope

Add tests for:

- request ID generation
- existing request ID propagation
- timeout behaviour
- panic recovery
- oversized request rejection
- error response schema

Update the REST contract specification for health and error endpoints.
```

## Prompt 1.4: Phase 1 review

Use the standard review prompt and phase-gate prompt.

### Phase 1 acceptance

```text
- Platform API starts and stops cleanly.
- Liveness and readiness endpoints behave as documented.
- Structured logs contain request and service context.
- Unit tests and static analysis pass.
- Container builds successfully.
```

---

# 4. Phase 2: PostgreSQL persistence and migrations

## Goal

Introduce PostgreSQL as the authoritative transactional store with safe migrations and repository boundaries.

## Prompt 2.1: Design persistence layer

```text
Design Phase 2 persistence before implementing it.

Read:

- specs/05-data/postgres-schema.md
- specs/05-data/indexing.md
- specs/05-data/migrations.md
- specs/02-domain/domain-model.md
- specs/07-security/tenant-isolation.md

Produce:

- initial schema boundary
- migration-tool decision
- connection-pool settings
- transaction abstraction
- repository interfaces
- tenant scoping rules
- test database strategy
- backup implications
- rollback limitations

Record significant choices in ADRs when needed.

Do not write persistence code yet.
```

## Prompt 2.2: Add database bootstrap

```text
Implement PostgreSQL connectivity and migration infrastructure.

Scope:

- database configuration
- bounded connection pool
- startup connectivity validation
- readiness integration
- migration command
- initial migration tracking
- local PostgreSQL through Docker Compose
- integration-test database setup
- database metrics hooks for later instrumentation

Add tests for:

- successful connection
- invalid configuration
- unavailable database readiness
- migration application
- repeated migration safety

Do not create domain tables beyond migration infrastructure in this task.
```

## Prompt 2.3: Implement tenants and projects

```text
Implement the Tenant and Project persistence vertical slice.

Read the approved domain and API specifications.

Implement:

- tenants table
- projects table
- required constraints
- tenant-prefixed indexes
- domain entities
- repository interfaces
- PostgreSQL adapters
- project creation use case
- project retrieval use case
- REST endpoints required by the spec
- transactional tests
- tenant isolation tests

Requirements:

- Never fetch a project by project ID without tenant scope.
- Use database constraints for invariants where possible.
- Return stable domain errors, not raw SQL errors.
- Do not add caching.
```

## Prompt 2.4: Enable PostgreSQL row-level security

```text
Implement and test PostgreSQL row-level security for tenant-owned tables.

Requirements:

- document application tenant-context setup
- enable RLS on relevant tables
- create least-privilege application role
- test that cross-tenant reads and writes fail
- test transaction-pool behaviour
- ensure migrations can still run with a privileged migration role
- update local-development documentation

Do not rely on RLS as the only isolation layer. Repository queries must remain
tenant scoped.
```

### Phase 2 acceptance

```text
- Migrations are repeatable.
- Projects are tenant isolated at application and database layers.
- Database readiness is accurate.
- Integration tests run from one documented command.
```

---

# 5. Phase 3: Agent run domain and state machine

## Goal

Implement durable run creation and explicit workflow state transitions.

## Prompt 3.1: Validate state-machine specification

```text
Review the AgentRun state machine before implementation.

Read:

- specs/02-domain/agent-run-state-machine.md
- specs/02-domain/invariants.md
- specs/02-domain/error-taxonomy.md
- specs/03-services/platform-api.md

Identify:

- missing transitions
- ambiguous retry semantics
- terminal states
- cancellation races
- timeout semantics
- optimistic-locking requirements
- actor permissions
- required audit events

Propose corrections in the specification.
Do not implement until the state machine is internally consistent.
```

## Prompt 3.2: Implement AgentRun persistence

```text
Implement AgentRun and AgentRunAttempt persistence.

Include:

- migrations
- entities and value objects
- status enum
- transition validation
- optimistic version column
- idempotency key uniqueness per tenant
- repository methods
- creation transaction
- retrieval
- list with pagination
- tests for every allowed and prohibited transition

Do not add Kafka or scheduling yet.
```

## Prompt 3.3: Implement create-run API

```text
Implement the create AgentRun API vertical slice.

Requirements:

- authenticate with a temporary development identity abstraction
- derive tenant from identity context
- validate prompt metadata and resource request
- enforce idempotency key semantics
- store prompt by reference rather than unrestricted log output
- persist run in QUEUED state
- return 202 Accepted
- expose GET run status
- add OpenAPI contract
- add contract and integration tests

Do not implement actual asynchronous execution.
```

## Prompt 3.4: Implement cancel command semantics

```text
Implement cancellation-request semantics at the domain and API layers.

Scope:

- POST /v1/runs/{id}/cancel
- transition to CANCELLING when valid
- idempotent repeated cancellation
- terminal-state behaviour
- optimistic concurrency handling
- audit-event placeholder persisted transactionally if already specified
- tests for concurrent completion versus cancellation

Do not terminate Kubernetes workloads because they do not exist yet.
Document the future operator responsibility.
```

### Phase 3 acceptance

```text
- Run creation is idempotent.
- State transitions are explicit and tested.
- Concurrent updates do not silently overwrite state.
- Cancellation semantics are documented.
```

---

# 6. Phase 4: Transactional outbox and Kafka backbone

## Goal

Add reliable domain-event publication and idempotent consumption.

## Prompt 4.1: Design event architecture

```text
Design the initial event architecture.

Read:

- specs/04-contracts/event-envelope.md
- specs/04-contracts/kafka-topics.md
- specs/01-architecture/failure-model.md
- ADRs related to PostgreSQL and Kafka

Produce:

- event envelope
- topic naming
- partition keys
- schema versioning
- producer retry policy
- consumer acknowledgement model
- processed-event storage
- retry and DLQ approach
- local Redpanda or Kafka setup
- test strategy

Resolve contradictions in specifications before implementation.
```

## Prompt 4.2: Implement transactional outbox

```text
Implement the transactional outbox.

Scope:

- outbox_events migration
- outbox domain model
- transaction-aware event insertion
- create-run event insertion in the same transaction
- relay claiming with FOR UPDATE SKIP LOCKED
- bounded batch publication
- retry metadata
- publication status
- cleanup/retention behaviour
- metrics hooks
- integration tests simulating broker failure

Requirements:

- database commits must not depend on immediate Kafka availability
- duplicate publication must be tolerated
- relay instances must safely compete
```

## Prompt 4.3: Implement Kafka adapter and event schemas

```text
Implement the Kafka/Redpanda infrastructure adapter.

Include:

- producer
- consumer-group base
- event serialization
- envelope validation
- schema-version rejection policy
- trace and request metadata propagation
- graceful shutdown
- local development configuration
- contract tests

Do not implement business consumers in this task.
```

## Prompt 4.4: Implement idempotent consumer foundation

```text
Implement the idempotent-consumer foundation.

Include:

- processed_events migration
- transaction helper for checking and recording event IDs
- duplicate-event behaviour
- consumer retry classification
- poison-message handling
- DLQ publication metadata
- tests proving a crash after local commit does not duplicate business effects

Create a reusable but explicit abstraction. Do not hide business transactions
behind an over-general generic framework.
```

### Phase 4 acceptance

```text
- Run creation writes an outbox event atomically.
- Broker downtime does not lose events.
- Duplicate messages do not duplicate business effects.
- Event schemas are versioned and validated.
```

---

# 7. Phase 5: Scheduler

## Goal

Claim queued runs fairly, enforce limits, select clusters, and create execution intent.

## Prompt 5.1: Scheduler implementation plan

```text
Create a detailed implementation plan for the Scheduler.

Read:

- specs/03-services/scheduler.md
- state machine
- quota requirements
- Kafka contracts
- database schema
- failure model
- observability requirements

Cover:

- queue claiming
- scheduler leases
- fairness
- tenant quotas
- capacity reservation
- cluster selection strategies
- retries
- leader election versus competing consumers
- metrics
- integration tests

Write the plan and do not implement yet.
```

## Prompt 5.2: Implement queued-run claiming

```text
Implement scheduler queued-run claiming.

Requirements:

- claim eligible QUEUED runs using FOR UPDATE SKIP LOCKED
- order by priority descending and age ascending
- set scheduler lease atomically
- configurable batch size
- reclaim expired leases
- preserve tenant scope
- concurrent scheduler integration tests
- metrics for claim attempts and queue age

Do not select clusters yet.
```

## Prompt 5.3: Implement quota evaluation

```text
Implement scheduler eligibility and quota evaluation.

Use composable domain specifications for:

- tenant concurrent-run limit
- user concurrent-run limit
- queued-run limit
- CPU quota
- memory quota
- daily budget reservation
- supported runtime

Requirements:

- explain rejection or deferral reasons
- do not permanently fail capacity-related deferrals
- record quota decisions
- add tests for noisy-neighbour scenarios
```

## Prompt 5.4: Implement cluster-selection strategy

```text
Implement cluster selection behind a Strategy interface.

Initial strategies:

- least-loaded
- region-affinity with least-loaded fallback

Cluster data may initially come from a database-backed registry or deterministic
test source defined by the spec.

Add:

- deterministic scoring
- tie-breaking
- capacity checks
- unavailable-cluster exclusion
- decision logging
- tests

Do not create Kubernetes resources yet.
```

## Prompt 5.5: Emit scheduling intent

```text
Complete the scheduler workflow.

When a run is eligible and a cluster is selected:

- transition it to PROVISIONING atomically
- assign cluster
- create or update run attempt
- insert agent-run.scheduled outbox event
- release or expire lease correctly
- handle optimistic conflict
- preserve idempotency

Add end-to-end integration tests from queued run through scheduling event.
```

### Phase 5 acceptance

```text
- Multiple schedulers cannot schedule the same run attempt.
- Tenant quotas are enforced.
- Cluster selection is deterministic and replaceable.
- Scheduling actions are durably published.
```

---

# 8. Phase 6: Kubernetes CRD and Operator

## Goal

Translate scheduled run intent into reconciled Kubernetes resources.

## Prompt 6.1: Bootstrap operator

```text
Bootstrap the Kubernetes operator using Kubebuilder/controller-runtime or the
approved equivalent.

Implement only:

- operator module
- AgentRun API group and version
- CRD generation
- basic manager startup
- health and readiness
- leader election configuration
- envtest foundation
- nested operator AGENTS.md

Ensure generated files are reproducible and documented.
```

## Prompt 6.2: Define AgentRun CRD contract

```text
Implement and validate the AgentRun CRD from the specification.

Include:

- desired execution configuration
- resource requests and limits
- timeout
- retry policy
- workspace configuration
- network profile
- artifact destination references
- status phase
- conditions
- observedGeneration
- attempt metadata
- timestamps

Add:

- OpenAPI validation
- defaults where safe
- prohibited combinations
- conversion strategy notes for future versions
- CRD schema tests
```

## Prompt 6.3: Implement reconciliation foundation

```text
Implement the AgentRun reconciler foundation.

Responsibilities:

- fetch resource
- handle not found
- initialize status
- set observed generation
- create/update conditions
- ensure finalizer
- classify transient and permanent errors
- requeue with bounded backoff
- emit reconciliation metrics
- use deterministic resource names

Do not create execution Jobs yet.
```

## Prompt 6.4: Build execution resources

```text
Implement Kubernetes resource builders for an AgentRun.

Build:

- ServiceAccount
- ConfigMap
- Secret references without copying raw secrets
- PVC
- NetworkPolicy
- Job
- labels and annotations
- owner references
- restricted security context
- active deadline
- resource limits
- termination grace period

Use Builder or focused pure functions.

Add golden-manifest tests and security tests.
```

## Prompt 6.5: Reconcile Job lifecycle

```text
Extend the AgentRun reconciler to manage execution Job lifecycle.

Handle:

- resource creation
- already-existing resources
- Pod pending
- Job active
- Job succeeded
- Job failed
- OOMKilled
- eviction
- image pull failure
- deadline exceeded
- status updates
- retryable versus permanent failure
- duplicate reconciliation

Add envtest and kind-based integration tests.
```

## Prompt 6.6: Finalizer and cleanup

```text
Implement deletion and cleanup behaviour.

Requirements:

- use owner references for cluster-local resources
- use finalizer for external or explicitly retained resources
- make cleanup idempotent
- handle partial cleanup failures
- expose CleanupPending condition
- never leave a finalizer permanently blocked without diagnostics
- test deletion during provisioning and running states
```

### Phase 6 acceptance

```text
- Creating an AgentRun CR creates a secure Job.
- Reconciliation is idempotent.
- Status accurately reflects workload state.
- Retry and cleanup behaviour are tested.
```

---

# 9. Phase 7: Agent Runner

## Goal

Run a deterministic coding-agent simulator with trajectory, heartbeats, tests, and artifact output.

## Prompt 7.1: Design runner protocol

```text
Design the Agent Runner protocol.

Define:

- task input contract
- configuration loading
- workspace lifecycle
- trajectory event types
- heartbeat contract
- cancellation handling
- command execution policy
- test-result format
- artifact manifest
- final result contract
- exit-code taxonomy

Do not integrate a real LLM yet.
```

## Prompt 7.2: Implement deterministic runner

```text
Implement a deterministic Agent Runner.

It should:

- load a task
- initialize a workspace
- create a small application from controlled templates
- run configured tests
- emit ordered trajectory events
- emit periodic heartbeats
- produce a result manifest
- handle SIGTERM
- stop at deadline
- redact sensitive values from logs

Add unit and container integration tests.
```

## Prompt 7.3: Implement command executor

```text
Implement the restricted command-execution subsystem.

Requirements:

- explicit command model
- working-directory confinement
- timeout
- stdout/stderr size limits
- exit-code capture
- process-group termination
- environment allowlist
- prohibited command policy
- trajectory recording
- no shell interpolation unless explicitly required and safely handled

Add adversarial tests for traversal, oversized output, hung processes, and
cancellation.
```

## Prompt 7.4: Implement artifact upload

```text
Implement the Agent Runner artifact contract and upload client using the current
artifact-store abstraction.

Initially support local/MinIO-compatible object storage.

Upload:

- source snapshot
- test report
- trajectory chunks
- stdout/stderr references
- final result manifest

Requirements:

- content hashes
- deterministic object keys
- retryable upload
- partial-upload detection
- no success state before mandatory artifacts are durable
```

### Phase 7 acceptance

```text
- AgentRun Job can execute the deterministic runner.
- Heartbeats and trajectory are visible.
- Mandatory artifacts survive Pod deletion.
- Cancellation terminates child processes.
```

---

# 10. Phase 8: Object storage and artifact service

## Goal

Provide tenant-scoped durable storage and controlled access to large outputs.

## Prompt 8.1: Implement artifact-store port and adapters

```text
Implement the ArtifactStore port.

Adapters:

- local filesystem for unit tests
- MinIO/S3-compatible adapter for local integration

Capabilities:

- put
- get
- head
- delete
- presigned upload/download
- multipart upload where justified
- metadata and checksum validation

All keys must be tenant/project/run scoped.

Add conformance tests that run against every adapter.
```

## Prompt 8.2: Implement artifact metadata API

```text
Implement artifact metadata persistence and access APIs.

Requirements:

- metadata in PostgreSQL
- large content only in object storage
- tenant-scoped access
- content type and size
- checksum
- retention class
- actor audit
- short-lived presigned URLs
- no raw bucket credentials in clients

Add authorization and cross-tenant tests.
```

### Phase 8 acceptance

```text
- Artifacts are durable and checksum verified.
- Presigned access is short lived and tenant scoped.
- Object keys never collide across tenants.
```

---

# 11. Phase 9: Model Gateway and real LLM integration

## Goal

Introduce provider-neutral model access with budgets, retries, cost tracking, and safe credentials.

## Prompt 9.1: Design Model Gateway

```text
Design the Model Gateway before coding.

Define:

- internal API
- workload authentication
- provider adapter interface
- request and response model
- streaming policy
- timeout hierarchy
- retry classification
- rate limiting
- budget reservation
- token and cost accounting
- prompt/log redaction
- fallback strategy
- circuit breaker
- provider-specific anti-corruption layer
- test doubles

Create or update the Model Gateway specification and ADRs.
```

## Prompt 9.2: Implement gateway foundation with mock provider

```text
Implement the Model Gateway foundation using only a mock provider.

Include:

- internal authenticated endpoint
- provider interface
- mock provider
- request validation
- per-run attribution
- token usage response
- structured errors
- timeouts
- metrics
- tracing hooks
- unit and integration tests

Do not connect an external provider yet.
```

## Prompt 9.3: Implement OpenAI provider adapter

```text
Implement the approved OpenAI provider adapter behind the ModelClient port.

Requirements:

- use the current official API contract selected by the project
- secrets remain in the gateway
- explicit timeout
- retry only classified transient failures
- handle rate limits
- capture provider request ID
- normalize usage
- redact sensitive content
- support deterministic fake tests
- no provider-specific types outside the adapter

Update provider documentation and configuration examples.
```

## Prompt 9.4: Add rate limiting, circuit breaker, and fallback

```text
Implement resilience policies around model providers.

Add:

- per-tenant token bucket
- per-provider concurrency bulkhead
- retry with exponential backoff and jitter
- circuit breaker
- fallback strategy controlled by policy
- budget-aware refusal
- metrics and alerts for each mechanism

Tests must prove:

- permanent errors are not retried
- open circuit fails fast
- one tenant cannot consume all concurrency
- fallback does not double-charge successful requests
```

## Prompt 9.5: Integrate runner with gateway

```text
Add an LLMCodingAgent implementation to the Agent Runner.

Keep the deterministic TemplateAgent.

Requirements:

- select agent strategy by task configuration
- communicate only through Model Gateway
- preserve trajectory and cost attribution
- enforce maximum steps and token budget
- stop on cancellation
- reject unsupported tool calls
- retain deterministic tests with mock gateway
- gate real-provider tests behind explicit environment configuration
```

### Phase 9 acceptance

```text
- Agent Runner can use a real provider without receiving provider credentials.
- Usage and cost are attributed to a run.
- Rate limits, budgets, circuit breakers, and fallback are tested.
```

---

# 12. Phase 10: Build Service and software supply chain

## Goal

Build generated source into immutable, scanned, signed container images.

## Prompt 10.1: Design build pipeline

```text
Design the Build Service.

Cover:

- build.requested contract
- source retrieval
- isolated build execution
- rootless BuildKit or approved builder
- base image policy
- dependency network policy
- build cache
- image naming
- immutable digest
- SBOM
- vulnerability scan
- secret scan
- signature
- failure taxonomy
- retries
- artifact retention

Record architectural decisions before implementation.
```

## Prompt 10.2: Implement build orchestration

```text
Implement Build Service event consumption and durable build state.

Include:

- builds migration
- idempotent build.requested consumer
- build state machine
- Kubernetes Build Job creation abstraction
- deterministic Job names
- build status reconciliation
- outbox events for completed and failed builds
- integration tests

Use a fake builder first.
```

## Prompt 10.3: Implement rootless image build

```text
Implement the real isolated image build.

Requirements:

- no host Docker socket
- rootless approved builder
- source fetched by scoped credentials
- trusted base-image allowlist
- resource and time limits
- immutable image digest
- build logs stored as artifacts
- cache isolated by trust boundary
- reproducible build metadata where practical
```

## Prompt 10.4: Add scanning, SBOM, and signing

```text
Extend the build pipeline with:

- SBOM generation
- vulnerability scanning
- secret scanning
- policy threshold
- image signing
- provenance metadata
- artifact persistence
- explicit policy-rejected terminal state

Add tests using controlled vulnerable and clean fixture images.
```

### Phase 10 acceptance

```text
- Successful source produces an immutable image digest.
- Vulnerable or policy-violating images are rejected.
- SBOM, scan result, and provenance are retained.
- Build workers do not require host Docker access.
```

---

# 13. Phase 11: Deployment Service and GitOps

## Goal

Deploy approved images through Git and Argo CD, with rollback and environment policy.

## Prompt 11.1: Design deployment workflow

```text
Design deployment orchestration.

Define:

- deployment state machine
- environment promotion rules
- approval rules
- manifest generation
- Git repository layout
- commit metadata
- Argo CD application model
- status mapping
- timeout
- rollback
- concurrent deployment prevention
- drift handling
- DNS and TLS integration boundaries

Update ADRs before coding.
```

## Prompt 11.2: Implement deployment persistence and commands

```text
Implement deployments and deployment revisions.

Include:

- migrations
- create deployment command
- environment validation
- approval state
- project/environment concurrency control
- immutable build digest requirement
- audit event
- outbox event
- REST command and query APIs
- tests
```

## Prompt 11.3: Implement GitOps repository adapter

```text
Implement the GitOps repository adapter.

Capabilities:

- render approved manifests
- update project/environment path
- use image digest
- create deterministic commit message
- prevent lost updates
- detect conflicting deployment
- revert a revision
- test with local Git repository fixture

Do not call Argo CD directly to mutate Deployments.
```

## Prompt 11.4: Integrate Argo CD status

```text
Implement the Argo CD anti-corruption adapter.

Map external sync and health states to internal deployment states.

Track:

- desired revision
- observed revision
- sync status
- health status
- failing resource summary
- rollout timeout

Add tests for every documented Argo state combination.
```

## Prompt 11.5: Implement rollback

```text
Implement deployment rollback.

Requirements:

- rollback selects a known healthy immutable revision
- creates a new auditable Git commit
- does not rewrite Git history
- respects production approval policy
- is idempotent
- updates deployment state
- emits lifecycle events
- includes failure recovery tests
```

### Phase 11 acceptance

```text
- A successful build can be deployed through GitOps.
- Argo CD state is reflected in AgentForge.
- Rollback creates a new desired-state revision.
- Direct production mutation is not the standard path.
```

---

# 14. Phase 12: Authentication, authorization, and secrets

## Goal

Replace development identity with production-grade OIDC, RBAC, workload identity, and scoped secret access.

## Prompt 12.1: Threat-model review

```text
Update the project threat model based on the implemented system.

Review threats involving:

- malicious prompts
- malicious repositories
- command execution
- model provider access
- artifact access
- build supply chain
- Kubernetes API access
- metadata services
- cross-tenant leakage
- GitOps credentials
- cost abuse
- audit tampering

Rank risks and map each to preventive, detective, and recovery controls.
Do not implement controls in this prompt.
```

## Prompt 12.2: Implement OIDC authentication

```text
Implement OIDC authentication for public APIs.

Requirements:

- issuer and audience validation
- signature and key rotation handling
- expiration and not-before validation
- stable identity mapping
- tenant membership lookup
- no trust in client-supplied tenant headers
- test token fixtures
- development mode explicitly separated and disabled by default outside local
```

## Prompt 12.3: Implement authorization policy

```text
Implement resource-level RBAC.

Roles:

- organization owner
- project admin
- developer
- viewer
- platform operator
- security auditor

Requirements:

- explicit permission mapping
- service-layer enforcement
- tenant and project scope
- deny by default
- audit denied privileged attempts
- authorization tests for every public command
```

## Prompt 12.4: Implement secret broker/workload identity

```text
Implement the approved secret-access architecture.

Requirements:

- applications store secret references, not values
- workloads authenticate with workload identity
- broker verifies tenant, project, run, and purpose
- credentials are short lived and minimally scoped
- raw secrets are not logged
- access is audited
- agent workloads cannot list arbitrary secrets
- tests cover cross-tenant and expired-credential attacks
```

### Phase 12 acceptance

```text
- Public APIs authenticate through OIDC.
- Every privileged action has explicit authorization.
- Agent workloads receive no static platform credential.
- Secret access is scoped and audited.
```

---

# 15. Phase 13: Observability

## Goal

Deliver complete metrics, logs, traces, dashboards, SLOs, and actionable alerts.

## Prompt 13.1: Instrumentation audit

```text
Audit current observability against the specifications.

For each service and workflow, list:

- missing RED metrics
- missing USE metrics
- missing business metrics
- missing structured log fields
- missing trace propagation
- high-cardinality risks
- sensitive-data risks
- missing health indicators
- missing runbook links

Write the gap analysis before editing code.
```

## Prompt 13.2: Add OpenTelemetry tracing

```text
Implement end-to-end OpenTelemetry tracing.

Propagate context through:

- HTTP
- PostgreSQL spans
- Kafka headers
- scheduler
- Kubernetes resource annotations where useful
- Agent Runner
- Model Gateway
- Build Service
- Deployment Service

Requirements:

- bounded attributes
- no prompts, source code, tokens, or secrets in spans
- sampling configuration
- trace IDs in logs
- integration test proving one workflow trace remains connected
```

## Prompt 13.3: Implement metrics

```text
Implement the metrics specification.

Include:

- API RED metrics
- database pool metrics
- Kafka producer and consumer metrics
- queue depth and delay
- scheduler decisions
- controller reconcile metrics
- run lifecycle duration
- model tokens and cost
- build and deployment duration
- failure classifications
- artifact operations

Add metric-name and label-cardinality tests where practical.
```

## Prompt 13.4: Implement logging and Loki configuration

```text
Normalize structured logging across services.

Requirements:

- common fields
- error classification
- correlation IDs
- tenant/project/run IDs where authorized
- no secrets, raw prompts, or source contents
- Loki low-cardinality label policy
- retention configuration
- sample investigation queries

Add redaction tests.
```

## Prompt 13.5: Dashboards, SLOs, and alerts

```text
Create Grafana dashboards, SLO definitions, Prometheus alert rules, and runbook
links.

Dashboards:

- platform overview
- run lifecycle
- scheduler
- Kubernetes execution
- model gateway
- builds
- deployments
- cost

Alerts must be:

- symptom based
- actionable
- severity classified
- resistant to transient noise
- linked to a runbook

Add rule-validation tests.
```

### Phase 13 acceptance

```text
- One run can be followed through metrics, logs, and traces.
- Alerts have runbooks.
- Sensitive content is redacted.
- SLOs are measurable from implemented telemetry.
```

---

# 16. Phase 14: Usage ledger, budgets, and cost attribution

## Goal

Track model and infrastructure cost through an append-only ledger and enforce budgets safely.

## Prompt 14.1: Implement usage ledger

```text
Implement the append-only usage ledger.

Record:

- model input/output/cached tokens
- CPU seconds
- memory GB-seconds
- storage
- build duration
- artifact storage
- network egress when available

Requirements:

- source event idempotency
- unit and price version
- currency
- observed time
- tenant/project/run attribution
- corrections through compensating entries, not destructive edits
```

## Prompt 14.2: Implement budget reservation

```text
Implement budget reservation and settlement.

Flow:

- estimate before run/model/build
- reserve budget
- consume actual usage
- release unused reservation
- reject or pause when limit is exceeded
- handle expired reservations
- prevent concurrent overspending

Add transaction and concurrency tests.
```

## Prompt 14.3: Cost dashboards and alerts

```text
Implement cost projections and alerts.

Include:

- cost per successful run
- cost by tenant/project/model
- daily burn
- budget remaining
- anomalous cost spike
- abandoned workload cost
- top cost failure categories

Document approximation limits.
```

### Phase 14 acceptance

```text
- Usage entries are immutable and idempotent.
- Concurrent runs cannot bypass budgets.
- Cost can be explained from ledger entries.
```

---

# 17. Phase 15: Edge, networking, DNS, and TLS

## Goal

Expose generated applications safely and exercise real networking operations.

## Prompt 15.1: Network architecture audit

```text
Review and finalize the network architecture.

Cover:

- cluster DNS
- service discovery
- ingress
- TLS
- cert-manager
- Cloudflare or approved edge
- application hostname allocation
- egress controls
- metadata-service blocking
- private service access
- network policies
- CDN cache rules
- rate limits
- health checks
- WebSocket/SSE support

Record trust boundaries and packet paths.
```

## Prompt 15.2: Implement application ingress and TLS

```text
Implement generated-application ingress.

Requirements:

- deterministic tenant-safe hostname
- Ingress or Gateway API manifests
- TLS through cert-manager
- health-check route
- secure defaults
- no wildcard tenant confusion
- deployment-status integration
- local development equivalent
```

## Prompt 15.3: Implement edge adapter

```text
Implement the Cloudflare or approved edge adapter.

Capabilities:

- DNS record lifecycle
- proxy setting
- cache policy
- rate-limit policy
- purge
- idempotent reconciliation
- least-privilege token use
- audit events

Use an adapter and mock tests. Keep edge-provider types out of domain code.
```

## Prompt 15.4: Networking failure laboratory

```text
Create a networking failure laboratory with scripts and runbooks for:

- wrong Service selector
- wrong target port
- DNS failure
- NetworkPolicy denial
- expired or invalid certificate
- bad Ingress route
- CDN caching dynamic errors
- load-balancer health mismatch
- connection timeout

Each scenario must include:

- setup
- symptom
- diagnostic commands
- expected evidence
- repair
- prevention
- cleanup
```

### Phase 15 acceptance

```text
- Generated applications receive HTTPS hostnames.
- DNS and edge changes reconcile idempotently.
- Network failures can be reproduced and diagnosed.
```

---

# 18. Phase 16: Terraform and cloud deployment

## Goal

Provision a complete development cloud environment reproducibly.

## Prompt 16.1: Cloud decision ADR

```text
Choose the first supported cloud, GCP or AWS, based on the existing project goals.

Write an ADR comparing:

- managed Kubernetes
- database
- Redis
- Kafka/message service
- object storage
- registry
- IAM/workload identity
- DNS
- cost
- local-to-cloud portability

Choose only one initial cloud.
Do not implement both.
```

## Prompt 16.2: Terraform foundation

```text
Create the Terraform foundation.

Requirements:

- provider configuration
- remote-state design
- state locking
- environment separation
- naming and tagging
- input validation
- sensitive outputs
- reusable modules only where repetition exists
- plan and validate CI
- no embedded credentials

Start with networking and cluster prerequisites.
```

## Prompt 16.3: Provision managed services

```text
Implement Terraform for the approved cloud services:

- Kubernetes cluster
- PostgreSQL
- Redis
- object storage
- registry
- DNS integration
- workload identity
- secret manager
- observability prerequisites

Requirements:

- private networking where practical
- least privilege
- encryption
- backups
- deletion protection by environment
- cost-conscious development defaults
```

## Prompt 16.4: Bootstrap GitOps into cloud cluster

```text
Implement cluster bootstrap.

Include:

- Argo CD installation
- platform application root
- namespaces
- policy controller
- cert-manager
- observability stack
- secret integration
- environment overlays

The cluster should converge from Git after minimal bootstrap.
Document disaster recreation.
```

### Phase 16 acceptance

```text
- Terraform plan is reviewable and repeatable.
- A clean cloud environment can be provisioned.
- GitOps converges platform workloads.
- Static cloud credentials are not placed in Pods.
```

---

# 19. Phase 17: Reliability, scaling, and backpressure

## Goal

Harden the platform against overload and dependency failure.

## Prompt 17.1: Failure-mode audit

```text
Perform a failure-mode and effects analysis of the implemented platform.

For each dependency and service, record:

- failure
- user-visible effect
- detection
- retry behaviour
- data consistency impact
- fallback
- recovery
- blast radius
- test coverage

Prioritize the top ten unmitigated risks.
```

## Prompt 17.2: Implement backpressure and load shedding

```text
Implement backpressure and load-shedding policies.

Cover:

- API admission
- scheduler queue
- model provider
- build workers
- database saturation
- artifact uploads
- live log streams

Preserve:

- cancellation
- status reads
- incident operations
- critical deployment rollback

Add load tests proving bounded resource use.
```

## Prompt 17.3: Autoscaling

```text
Implement autoscaling policies.

Include:

- API HPA
- scheduler scaling from queue or consumer lag
- model gateway scaling
- build-worker scaling
- execution node autoscaling assumptions
- minimum availability
- maximum cost guardrails
- scale-down safety

Document why each signal is suitable.
```

## Prompt 17.4: Multi-AZ and disruption safety

```text
Harden platform services for disruption.

Add where relevant:

- topology spread
- PodDisruptionBudgets
- anti-affinity
- graceful termination
- connection draining
- leader-election safety
- database failover handling
- Kafka rebalance handling
- node-drain tests
```

### Phase 17 acceptance

```text
- Overload causes controlled degradation, not collapse.
- Critical operations remain available.
- Scaling signals and cost limits are documented.
- Node disruption does not corrupt workflow state.
```

---

# 20. Phase 18: Testing, chaos, and incident exercises

## Goal

Prove behaviour through automated tests and realistic incident simulations.

## Prompt 18.1: Test-coverage traceability audit

```text
Audit every approved requirement in TRACEABILITY.md.

For each requirement:

- locate implementation
- locate test evidence
- classify test level
- identify missing negative cases
- identify flaky or non-deterministic tests
- identify contracts without compatibility tests

Do not inflate coverage with trivial tests.
Produce a prioritized test backlog.
```

## Prompt 18.2: Load tests

```text
Implement load tests for:

- create-run API
- queued-run claiming
- Kafka event processing
- status reads
- live updates
- model gateway with fake provider
- artifact metadata
- deployment status

Measure:

- throughput
- p50/p95/p99
- error rate
- database pool saturation
- queue delay
- consumer lag
- memory growth

Define explicit pass/fail thresholds.
```

## Prompt 18.3: Chaos scenarios

```text
Implement automated or scripted chaos scenarios for:

1. Agent Pod OOMKilled
2. PostgreSQL connection exhaustion
3. Kafka outage and recovery
4. Redis outage
5. scheduler crash after claim
6. operator crash during reconciliation
7. object-store upload failure
8. Argo CD OutOfSync
9. node drain
10. model provider 429/503 spike

Each scenario must verify recovery invariants and clean itself up.
```

## Prompt 18.4: Incident simulations and postmortems

```text
Run the approved incident simulations.

For each:

- trigger fault
- capture detection time
- capture alert
- follow runbook
- restore service
- verify data consistency
- measure recovery
- write postmortem
- create corrective-action items

Do not claim an incident was successful without recorded evidence.
```

### Phase 18 acceptance

```text
- Critical requirements have test evidence.
- Load thresholds are explicit.
- Top failure modes are reproducible.
- Runbooks work during simulations.
```

---

# 21. Phase 19: Disaster recovery and operational readiness

## Goal

Prove that the platform can be operated, restored, upgraded, and handed over.

## Prompt 19.1: Backup and restore implementation

```text
Implement and verify backup and restore procedures for:

- PostgreSQL
- object storage
- GitOps repository assumptions
- secrets/configuration
- audit and usage data

Perform an actual restore into an isolated environment.
Record RPO, RTO, commands, evidence, and gaps.
```

## Prompt 19.2: Upgrade and migration drills

```text
Create and test upgrade procedures for:

- application services
- PostgreSQL schema
- AgentRun CRD
- Kubernetes version assumptions
- Argo CD
- observability stack
- Kafka schemas

Include backward compatibility, rollback limits, and staged rollout.
```

## Prompt 19.3: Operational readiness review

```text
Perform an operational readiness review.

Check:

- ownership
- on-call routing
- SLOs
- alerts
- dashboards
- runbooks
- capacity
- budgets
- backups
- restore evidence
- security controls
- dependency inventory
- certificate renewal
- vulnerability management
- deployment and rollback
- known limitations

Write:
specs/11-operations/operational-readiness-review.md
```

### Phase 19 acceptance

```text
- Restore has been tested, not merely documented.
- Upgrade paths exist.
- Operational ownership and runbooks are complete.
```

---

# 22. Phase 20: Portfolio polish and interview demonstration

## Goal

Make the project understandable, reproducible, and demonstrable without disguising unfinished work.

## Prompt 20.1: Documentation consistency audit

```text
Audit all documentation against implementation.

Find:

- stale paths
- stale commands
- code that contradicts specs
- implemented features marked incomplete
- incomplete features marked implemented
- obsolete ADR statuses
- missing diagrams
- missing examples
- undocumented limitations

Make documentation-only corrections first.
List code inconsistencies separately.
```

## Prompt 20.2: Create architecture documentation

```text
Create final architecture documentation.

Include:

- system context
- control/execution/application planes
- component diagram
- primary run sequence
- build and deployment sequence
- cancellation sequence
- data ownership
- trust boundaries
- failure recovery
- design pattern map
- scaling path
- deliberate trade-offs

Use Mermaid diagrams that render in GitHub.
```

## Prompt 20.3: Create reproducible demo

```text
Create a reproducible demonstration script.

The demo must:

1. start local dependencies
2. create a tenant and project
3. submit an agent run
4. show outbox and Kafka flow
5. show scheduler decision
6. show AgentRun CR and Job
7. stream trajectory
8. show artifacts
9. build and scan image
10. deploy through GitOps
11. show Argo CD health
12. show metrics/logs/traces
13. inject one failure
14. diagnose and recover
15. show cost attribution

Provide automated setup and cleanup where safe.
```

## Prompt 20.4: Create interview guide

```text
Create an interview guide derived from the real implementation.

For each major component include:

- problem
- design choice
- alternative considered
- failure mode
- scaling limit
- security concern
- observability
- test evidence
- likely interviewer questions
- concise answer grounded in code

Do not invent scale numbers or production claims.
```

## Prompt 20.5: Final repository audit

```text
Perform the final repository audit as:

- staff infrastructure engineer
- security reviewer
- SRE
- database reviewer
- Kubernetes operator reviewer
- hiring manager

Review independently, then synthesize.

Identify:

- release blockers
- portfolio blockers
- misleading claims
- missing evidence
- highest-value next improvements

Do not edit until findings are presented.
```

---

# 23. Reusable Codex prompts

## Start a new session

```text
Orient yourself to this AgentForge repository.

Read:

- AGENTS.md
- specs/SPEC-MANIFEST.md
- specs/PROJECT-STATUS.md
- specs/TRACEABILITY.md
- the nearest nested AGENTS.md for the target component

Then inspect the current branch and uncommitted changes.

Report:

- current milestone
- current implementation state
- applicable specifications
- open risks
- safest next task

Do not edit files.
```

## Resume a task

```text
Resume the current task from repository state, not from assumptions.

Inspect:

- git status
- git diff
- recent commits
- PROJECT-STATUS.md
- task plan
- affected tests

Summarize completed work, incomplete work, and failing validation.
Then continue only the originally defined scope.
```

## Ask Codex to generate a plan

```text
Create an implementation plan for the requested task.

The plan must include:

- governing specifications
- current implementation findings
- files to modify
- public-contract impact
- schema or migration impact
- concurrency and idempotency concerns
- security impact
- observability impact
- tests
- rollout and rollback
- explicit non-goals

Do not edit code.
```

## Ask Codex to implement from a plan

```text
Implement the approved plan exactly.

Work in small verifiable increments.

After each meaningful increment:

- compile
- run focused tests
- inspect the diff

Do not expand scope unless a blocking correctness issue requires it.
Record any such deviation explicitly.
```

## Ask Codex to debug a failing test

```text
Diagnose the failing test without weakening the assertion.

Steps:

1. Reproduce the failure.
2. Determine whether the defect is in implementation, test, fixture, environment,
   or specification.
3. Gather evidence.
4. Explain the root cause.
5. Apply the smallest correct fix.
6. Add regression coverage.
7. Run affected and neighbouring tests.

Do not add arbitrary sleeps or retries to hide races.
```

## Ask Codex to review security

```text
Perform a security review of the affected component.

Focus on:

- tenant-boundary violations
- authentication and authorization bypass
- secret exposure
- command injection
- path traversal
- SSRF
- metadata-service access
- Kubernetes privilege escalation
- unsafe deserialization
- supply-chain risks
- audit gaps
- denial of service
- cost abuse

Return exploitable scenarios, severity, evidence, and fixes.
```

## Ask Codex to review concurrency

```text
Review the implementation for concurrency and distributed-systems defects.

Check:

- lost updates
- duplicate processing
- stale reads
- lease expiry races
- transaction boundaries
- acknowledgement ordering
- retry safety
- cancellation races
- out-of-order events
- leader failover
- cache consistency
- partial failure

Use concrete interleavings to demonstrate any defect.
```

## Ask Codex to update docs after code

```text
Update documentation to match the completed implementation.

Review:

- component specification
- contracts
- ADRs
- TRACEABILITY.md
- PROJECT-STATUS.md
- runbooks
- examples
- local-development commands

Do not rewrite unrelated documentation.
Do not mark untested behaviour complete.
```

## Ask Codex to prepare a commit

```text
Prepare the current change for commit.

Tasks:

- inspect the full diff
- remove accidental changes
- run required validation
- ensure generated files are current
- ensure specs and traceability are updated
- summarize residual risks

Propose:

- conventional commit subject
- detailed commit body
- testing evidence

Do not commit until instructed.
```

## Ask Codex to prepare a pull request

```text
Prepare a pull request description for the current branch.

Include:

- problem
- approach
- architecture impact
- files/components changed
- contract or migration changes
- security considerations
- observability
- tests and evidence
- rollout
- rollback
- screenshots or demo commands when relevant
- known limitations

Use only claims supported by the repository and test results.
```

---

# 24. Parallel-agent prompts

Use parallel agents for independent analysis, not simultaneous edits to the same files.

## Coordinator prompt

```text
Coordinate a multi-agent review for the current task.

Assign independent roles:

1. architecture reviewer
2. implementation reviewer
3. test reviewer
4. security reviewer
5. operations reviewer

Each reviewer must inspect the same governing specifications but focus on its
assigned concern.

Do not allow parallel edits.

Collect findings, deduplicate them, resolve contradictions, and produce one
severity-ordered review.
```

## Parallel implementation split

```text
Split this milestone into non-overlapping worktrees or branches.

For each subtask specify:

- owned files
- prohibited files
- contract dependency
- merge order
- validation command
- handoff artifact

Do not split work that requires concurrent modification of the same schema,
state machine, or public contract.
```

---

# 25. Recommended session rhythm

For each task:

```text
1. Orientation prompt
2. Planning prompt
3. Human review of plan
4. Implementation prompt
5. Focused test prompt if needed
6. Standard review prompt
7. Fix prompt
8. Documentation update prompt
9. Commit preparation prompt
10. Phase gate when the phase ends
```

This is intentionally repetitive. Infrastructure correctness benefits from
boring discipline, despite the software industry repeatedly attempting to make
everything resemble an improvisational theatre exercise.

---

# 26. What not to prompt

Avoid:

```text
Build the whole platform.
```

```text
Make everything production ready.
```

```text
Implement all microservices and Kubernetes.
```

```text
Fix all bugs and improve the architecture.
```

```text
Continue until done.
```

Replace them with bounded tasks that identify:

- outcome
- relevant specifications
- scope
- constraints
- tests
- completion bar

---

# 27. First five prompts to run in a fresh repository

Run these in order:

## Prompt A

```text
Inspect this repository without modifying files.

Read all root-level documentation and locate the AgentForge specification
bundle.

Produce:
specs/12-llm-development/bootstrap-audit.md

Include repository inventory, specification gaps, contradictions, broken
references, and the recommended canonical structure.
```

## Prompt B

```text
Create or normalize the specification repository, manifest, project status,
traceability matrix, glossary, and root AGENTS.md according to the bootstrap
audit.

Do not create production code.
```

## Prompt C

```text
Create the minimal development harness:

- Makefile
- tool checks
- formatting
- linting
- repository verification
- GitHub Actions skeleton
- local prerequisite documentation

Do not add product dependencies.
```

## Prompt D

```text
Plan Phase 1, Platform API foundation.

Do not edit code.

Write a precise plan covering package structure, dependencies, health checks,
configuration, logging, graceful shutdown, tests, Dockerfile, and acceptance
criteria.
```

## Prompt E

```text
Implement the approved Phase 1 plan.

Add only the Platform API foundation, health endpoints, configuration,
structured logging, graceful shutdown, tests, Dockerfile, and Make targets.

Do not add databases, brokers, authentication, Kubernetes, or business APIs.
```
