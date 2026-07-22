# Phase 5 Plan: Scheduler

## Status and canonical sources

Phase 5 is approved to begin on the existing `phases/phase-5` branch. The paths
in the supplied prompt predate the repository manifest. Their canonical
equivalents are `04-services/02-scheduler.md`, `03-domain/02-state-machines.md`,
`09-security/02-identity-rbac-secrets.md`, `14-operations/03-capacity-cost.md`,
`07-events/02-topic-catalog.md`, `06-data/01-relational-schema.md`,
`02-architecture/03-quality-attributes.md`, `10-observability/01-telemetry-standards.md`,
and `12-testing/`.

This plan is design-only. Each implementation sub-phase is validated and
committed before the next begins. The existing root `docker-compose.yml` is the
only Compose definition and will be extended in place in Phase 5.8.

## Process boundary

The Scheduler is a distinct control-plane process and image, already justified
by the container architecture. It remains in the current Go control-plane
module as `services/platform-api/cmd/scheduler` with focused packages under
`internal/application/scheduler` and PostgreSQL adapters under
`internal/adapters/postgres`. This shares the established domain, event,
database, logging, and build-info contracts without prematurely extracting a
new module or duplicating internal packages. It receives its own Dockerfile,
configuration, lifecycle, health endpoints, and nested `AGENTS.md` in Phase
5.8.

PostgreSQL is authoritative. Kafka `agent-run.requested.v1` records are wake-up
hints, not queue ownership or workflow state. Every replica also performs a
periodic database scan, so missing, delayed, or duplicate hints cannot lose a
run. No leader election is used: replicas compete through database row locks,
leases, optimistic versions, quota locks, and unique reservations.

## Resolved contract decisions

1. A Scheduler claim moves `QUEUED` or due `CAPACITY_WAIT` to `SCHEDULING` and
   records an expiring lease, but emits no public `agent-run.scheduled.v1`
   event. A lease is temporary coordination, not a completed scheduling fact.
2. `agent-run.scheduled.v1` is inserted when the owned scheduling transaction
   selects a cluster, reserves capacity/budget, assigns the current attempt,
   and transitions the run to `PROVISIONING`. This matches the Phase 5 gate.
3. `agent-run.provisioning-requested.v1` remains a reserved, unimplemented
   contract until the Phase 6 Operator boundary decides whether it needs a
   separate command event. Phase 5 does not emit two facts for one transition.
4. The attachment mentions project and tenant status, but current aggregates
   have no status field. Phase 5.3 adds explicit bounded `ACTIVE`/`SUSPENDED`
   status columns with `ACTIVE` defaults and includes them in policy queries.
5. No storage quota is currently specified as a schedulable run input, so
   Phase 5 does not invent one. CPU, memory, concurrency, queue, and daily
   budget limits are implemented; storage remains a future contract change.
6. The existing run request has no preferred region or execution profile.
   Phase 5 migrations add a default `standard` profile and an optional preferred
   region. Existing rows remain compatible and Phase 3.3 may expose validated
   inputs later.

## Phase 5.2: queue claiming and leases

Migration `000005` adds run priority, execution profile, preferred region,
next-eligible time, scheduler lease owner/expiry, and bounded decision
code/reason fields. Priority is a bounded signed integer with default zero;
existing rows retain FIFO behavior.

The queue adapter uses one short PostgreSQL transaction:

1. Select eligible `QUEUED`, due `CAPACITY_WAIT`, or expired `SCHEDULING` rows.
2. Rank within a fairness round by effective priority descending and creation
   time ascending, then stable tenant/run IDs. Effective priority applies a
   bounded queue-age boost, preventing indefinite starvation. A row number per
   tenant interleaves tenants before a second run from one tenant is selected.
3. Lock the bounded candidates with `FOR UPDATE SKIP LOCKED`.
4. Conditionally set `SCHEDULING`, lease owner/expiry, updated time, and
   incremented aggregate version.
5. Commit before any quota, cluster, Kafka, or other external operation.

An unexpired lease cannot be stolen. The same owner may renew only an owned,
unexpired lease using the expected aggregate version. Expired leases are
reclaimable and increment the version. Batch size, lease duration, scan
interval, worker count, and aging interval are bounded configuration. Empty
queues return an empty slice and no error.

The cross-tenant claim path uses a dedicated `agentforge_scheduler` role with
`BYPASSRLS`, following the accepted relay isolation pattern. It receives only
the column-level reads and updates needed for scheduling, plus explicit grants
added by later scheduler migrations. It cannot read prompt references, request
hashes, cancellation text, or unrelated tenant data. Every claimed record
carries a validated tenant ID; every later mutation predicates both tenant and
run identity. Cross-tenant and transaction-local context tests are mandatory.

## Phase 5.3: eligibility and quotas

Focused domain policies return an `EligibilityDecision` containing outcome
(`ELIGIBLE`, `DEFER`, or `REJECT`), stable reason code, bounded safe explanation,
and optional next-eligible time. Permanent tenant/project suspension and
unsupported runtime/profile reject; capacity, concurrency, queue pressure, and
budget exhaustion defer. Decisions never include prompts, source, credentials,
or raw database errors.

Migration `000006` adds tenant/project statuses, scheduler policy rows, allowed
runtimes/profiles, and decision history. Tenant policies define bounded tenant
and per-user concurrent-run limits, queued-run limit, aggregate active CPU and
memory limits, and daily budget in integer minor units. The trusted actor stored
in `created_by` is the initial user key until the identity tables are
implemented. Missing policy fails closed for scheduling while bootstrap creates
an explicit local policy fixture.

Adapters use indexed aggregate SQL over active run/reservation statuses; they
never load a tenant's runs into memory. Evaluation locks the tenant policy row
to serialize concurrent decisions. Because state can change after a standalone
decision, Phase 5.7 repeats quota evaluation inside the final reservation and
transition transaction. Saturated tenants are deferred before worker capacity
is consumed, while other tenants remain claimable.

## Phase 5.4: cluster registry

Migration `000007` creates validated global `clusters`,
`cluster_capacity_snapshots`, and optional `cluster_tenant_allowlist` tables.
A cluster has immutable identity, region, status, maintenance flag, supported
runtimes/profiles, scheduling weight, integer cost attributes, heartbeat time,
and optimistic version. Capacity snapshots contain allocatable CPU/memory and
reported queued workload. Metadata arrays are bounded, normalized, and
non-empty; negative capacity, invalid regions/statuses, future-skewed
heartbeats, and unsafe identifiers are rejected.

The repository exposes register, heartbeat/update, get, and eligible-list
operations. A local internal CLI command supports deterministic registration
and updates; it is not a public tenant API. A cluster is selectable only when
`ACTIVE`, outside maintenance, heartbeat-fresh, runtime/profile-compatible,
tenant-allowed when restricted, and backed by a current capacity snapshot.

## Phase 5.5: replaceable deterministic selection

The application layer owns a small `Strategy` interface. Implementations are:

- `least-loaded`: exclude ineligible/insufficient clusters, then minimize the
  maximum of CPU and memory utilization after the proposed reservation;
  reported queued workload and approved integer cost/weight adjust the score.
- `region-affinity`: run the same score over preferred-region candidates and
  fall back to the complete eligible set only when that region has none.

Scores use integer basis points to avoid floating-point drift. Ties resolve by
lower projected utilization, lower queued workload, lower effective cost, then
cluster ID lexicographically. The selected strategy is bounded configuration.
No candidate produces a temporary `NO_CAPACITY` deferral, not a terminal run
failure. Decisions record strategy, score components, chosen cluster, and a
safe reason; logs use bounded fields and metrics avoid tenant/run labels.

## Phase 5.6: capacity and budget reservations

Migration `000008` creates tenant-owned `capacity_reservations` and
`budget_reservations`. A capacity reservation references exactly one current
run attempt and cluster, stores estimated CPU/memory, state, creation/expiry,
and settlement/release timestamps. A unique active run-attempt constraint and
locked cluster capacity row prevent double or over-reservation. Budget amounts
use integer minor units and a tenant/day boundary.

Reserve, release, settle, and bounded expired-reclaim operations are
idempotent. Release on scheduling failure occurs in the same transaction when
possible; committed reservations are released by an explicit recovery command.
Execution-start processing settles capacity ownership for later operator
accounting. Estimates protect scheduler admission but never replace Kubernetes
admission, ResourceQuota, or real node capacity.

## Phase 5.7: atomic scheduling intent

For an owned, unexpired lease, one PostgreSQL transaction:

1. Locks the run, current pending attempt, tenant policy, selected cluster
   capacity, and relevant reservation rows in a documented order.
2. Revalidates aggregate/attempt versions, cancellation, tenant/project state,
   quotas, cluster health/support/allowlist, and available capacity.
3. Creates idempotent capacity and budget reservations attached to the attempt.
4. Sets attempt cluster/profile/reservation metadata and moves it to `STARTING`.
5. Moves the run to `PROVISIONING`, records deterministic selection metadata,
   clears the scheduler lease/deferral, and increments both versions.
6. Inserts the versioned `agent-run.scheduled.v1` envelope and serialized
   payload into `outbox_events` with run ID as partition key.
7. Commits before returning success.

Exact replay observes the existing assignment/reservations/event and returns
the prior result. A changed assignment or stale version conflicts. If no cluster
is available, an owned transaction moves the run to `CAPACITY_WAIT`, clears the
lease, records a bounded reason/next eligibility, and emits
`agent-run.capacity-wait.v1` atomically. No path creates Kubernetes resources.

## Phase 5.8: process lifecycle and backpressure

The Scheduler executable validates configuration, opens the scheduler database
pool, starts bounded workers, consumes requested-event wake hints when Kafka is
configured, and performs periodic authoritative scans. Duplicate hints only
coalesce wake-ups. Polling uses bounded jitter/backoff after transient database
or broker errors. Workers stop claiming when their bounded channel is full;
leases are renewed only for active work and are allowed to expire after a crash.

Graceful shutdown stops hints and scans, stops new claims, cancels bounded
in-flight work, closes Kafka/database clients, and exits within the configured
deadline. A small HTTP server exposes `/health/live`, database-backed
`/health/ready`, and `/metrics`. Readiness fails when PostgreSQL is unavailable;
Kafka hint loss does not make the authoritative polling scheduler unready.

The root Compose file gains only a `scheduler` service and scheduler database
credential. `.env.example` documents every Phase 5 variable. The image is
non-root and contains only the scheduler binary. The nested `AGENTS.md` fixes
the service ownership, test, tenancy, and no-Kubernetes boundaries.

## Observability

Structured logs include service, owner, bounded decision/strategy code, queue
age bucket, cluster, and correlation fields; tenant/run IDs remain log fields,
not metric labels. Metrics cover queue depth/oldest age, claim attempts/results,
lease reclaim/renew/conflict, eligibility outcomes, quota dimension, cluster
exclusions, selection latency/outcome, reservation active/expired/released,
scheduling latency, worker saturation, and dependency errors. Traces propagate
from Kafka hints and use a new scheduling span for polling discoveries.

## Test and gate strategy

- Domain tests: decision explanations, every quota boundary, runtime/profile
  support, deterministic score/ties/fallback, and reservation state rules.
- PostgreSQL integration: two claimers, active/expired leases, priority/age and
  tenant interleaving, empty queue, optimistic conflict, connection restart,
  tenant context, concurrent quota evaluation, noisy neighbour isolation,
  stale/maintenance/allowlisted clusters, double/expired reservations, atomic
  assignment/outbox rollback, and crash recovery.
- Event contracts: schemas, compatibility baselines, canonical examples,
  producer output, and supported fixtures for both Scheduler event types.
- Component tests: bounded workers, poll fallback, duplicate Kafka wake-ups,
  backpressure, health/readiness, graceful shutdown, and root-Compose startup.
- Load tests: seeded queue mixes measure claim throughput, tenant service
  distribution, starvation bound, query count/latency, and worker saturation.
  Results are evidence only when run in a controlled environment; no scale
  claim is inferred from unit tests.

The final gate runs format, lint, unit, race, vet, repository verification,
PostgreSQL and Redpanda integration, Compose validation, event compatibility,
service image builds, concrete interleaving review, status/traceability updates,
and `15-llm-development/audits/phase-05-completion-audit.md`.

## Explicit exclusions

- No Kubernetes API calls, CRD creation, Jobs, or admission claims.
- No Redis authority or lock correctness dependency.
- No client-controlled tenant identity, cluster metadata, status, or quota.
- No storage quota until a storage request contract exists.
- No production capacity, fairness, SLO, cost-saving, or availability claim
  without later measured evidence.
