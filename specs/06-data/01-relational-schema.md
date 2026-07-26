# Relational Data Specification

## Primary tables

- tenants
- users
- tenant_memberships
- projects
- project_environments
- agent_runs
- agent_run_attempts
- builds
- deployments
- deployment_revisions
- budgets
- budget_reservations
- usage_ledger
- audit_events
- outbox_events
- processed_events
- idempotency_records
- clusters
- cluster_capacity_snapshots

## Rules

- Every tenant-owned table includes `tenant_id`.
- Foreign-key relationships include or validate tenant identity.
- Mutable aggregates include a `version` column.
- Timestamps use UTC `TIMESTAMPTZ`.
- IDs are UUIDv7 or another time-sortable globally unique identifier.
- Money uses integer minor units or fixed decimal, never floating point.
- High-volume append-only tables are partitioned by time.

## Phase 2 implementation boundary

`tenants` and `projects` are the first implemented tables. Tenant and project
identifiers are application-generated UUIDv7 values. `projects` has a
restricting tenant foreign key, tenant-scoped name uniqueness, UTC creation and
update timestamps, and a positive aggregate version. Its tenant-prefixed
creation index supports future project history queries.

`projects` has PostgreSQL RLS enabled and forced for the application role. The
application role receives tenant context only through transaction-local
`app.tenant_id`; repository SQL still includes `tenant_id` explicitly.

## Phase 3 persistence boundary

`agent_runs` and `agent_run_attempts` are the next implemented tenant-owned
tables. A run stores project ownership, a secure prompt reference (never raw
prompt text), runtime/resource/timeout constraints, max and current attempt
numbers, lifecycle status, a normalized failure category, optimistic version,
tenant-scoped idempotency key and effective-request hash, actor, cancellation
metadata, and UTC lifecycle timestamps. Phase 3.3 additionally stores the
safe immutable create-response snapshot used for idempotent replay; it contains
only response-visible run fields and deliberately excludes prompt references,
request hashes, and actor data. The composite project/tenant foreign key
prevents a run from referencing a project in another tenant.

An attempt stores its owning run and tenant, immutable monotonic attempt number,
lifecycle status, normalized failure category, optimistic version, opaque
cluster/workload references, and UTC timestamps. `(run_id, attempt_number)` is
unique. `agent_runs` has tenant/project history and partial scheduler-queue
indexes; attempts have a partial active-attempt index. Both tables have forced
RLS for `agentforge_app`, transaction-local tenant context, explicit repository
tenant predicates, and only `SELECT`, `INSERT`, and `UPDATE` grants.

## Phase 4.2 transactional outbox boundary

`outbox_events` durably records a canonical event envelope, routing metadata,
aggregate identity/version, publication disposition, lease ownership, retry
state, bounded failure details, broker acknowledgement metadata, and UTC
timestamps. AgentRun creation inserts `agent-run.requested.v1` into this table
inside the same tenant transaction as the run and first attempt. Event
serialization and validation happen before that transaction begins.

The claim index supports ordered, competing relay claims with
`FOR UPDATE SKIP LOCKED`; the published index supports bounded retention
cleanup. Relay claims use expiring leases and preserve the event ID across
retries and crash recovery. Cleanup deletes only `PUBLISHED` rows; pending and
terminal rows are never removed by the implemented cleanup operation.

Forced RLS remains enabled. `agentforge_app` can only insert rows accepted by
the transaction-local tenant policy. The isolated `agentforge_relay` role uses
`BYPASSRLS` solely to perform cross-tenant `SELECT`, `UPDATE`, and `DELETE` on
`outbox_events`; it has no grants on tenant business tables.

## Phase 4.4 processed-event boundary

`processed_events` records the stable consumer identity and event UUIDv7 as its
primary key, plus tenant, event/schema, aggregate, source topic/partition/offset,
processing time, and optional replay identity. A consumer inserts this marker
and applies its business effect inside one tenant transaction. A primary-key
conflict is an acknowledged duplicate no-op; a failed effect rolls the marker
back so delivery can retry.

The table has forced RLS and only tenant-scoped `SELECT`/`INSERT` grants for
`agentforge_app`. Its tenant/time index supports the future one-year retention
job, which remains disabled until replay policy is configured. Successful
markers are never deleted to force replay.

## Phase 5.2 scheduler queue boundary

Migration `000005_scheduler_queue_leases` adds bounded priority, execution
profile, optional preferred region, next-eligibility, Scheduler lease, and safe
decision fields to `agent_runs`. The partial queue index supports queued, due
capacity-wait, and expired-scheduling candidates ordered by priority and age.
Database constraints require lease owner/expiry together and only while a run
is `SCHEDULING`.

The isolated `agentforge_scheduler` role has `BYPASSRLS` solely because fair
claiming spans tenants. It receives column-level access to allowlisted run and
attempt scheduling metadata and cannot select prompt references, request
hashes, or cancellation text. Claims use `FOR UPDATE SKIP LOCKED`, short
transactions, expected aggregate versions, and expiring leases. Every returned
record retains its tenant ID for explicit tenant/run predicates downstream.

## Phase 5.3 eligibility and quota boundary

Migration `000006_scheduler_eligibility_quotas` adds explicit `ACTIVE` or
`SUSPENDED` tenant/project status, tenant scheduling policies, daily budget
usage, and append-only explained eligibility decisions. Policies bound tenant
and user concurrency, queue depth, aggregate CPU/memory, daily integer-minor-
unit budget, allowed runtimes/profiles, and deferral duration. Missing policy
fails closed as a temporary deferral.

Eligibility uses indexed aggregate SQL over active and waiting status sets; it
does not load run collections. The tenant policy row is locked during snapshot
evaluation and decision insertion. Decision rows preserve bounded outcome,
reason, explanation, aggregate counts, resource/budget usage, run version, and
next-eligibility time. Forced RLS remains enabled; only the isolated Scheduler
role receives the required cross-tenant reads and decision insert.

## Phase 5.4 execution-cluster registry boundary

Migration `000007_execution_cluster_registry` creates global `clusters`,
append-only `cluster_capacity_snapshots`, and tenant-scoped
`cluster_tenant_allowlist`. Cluster identifiers, regions, statuses, bounded
runtime/profile arrays, scheduling weights, non-negative integer costs,
heartbeat timestamps, and versions are database constrained. Capacity reports
are immutable per cluster and observation time and reject negative resources.

Only the isolated Scheduler role can register/update clusters and capacity or
manage allowlists. Candidate queries use the latest snapshot and require fresh
cluster and capacity timestamps, active/non-maintenance state, compatible
runtime/profile metadata, and an allowlist match for restricted clusters.

## Phase 5.6 reservation boundary

Migration `000008_scheduler_reservations` creates tenant/run/attempt-owned
capacity and budget reservation ledgers. Partial unique indexes permit only one
active reservation per run attempt. Cluster/expiry and tenant/expiry indexes
support admission totals and bounded reclaim. Lifecycle checks require exactly
the settlement or release timestamp appropriate to each state.

The Scheduler role alone may create and transition reservations and adjust the
reserved/spent columns of daily budget usage. Admission serializes on the
tenant policy and cluster rows before checking latest capacity, existing active
reservations, and locked daily usage. Historical released, expired, and settled
records remain immutable evidence.

## Phase 5.7 atomic intent boundary

Migration `000009_atomic_scheduling_intent` attaches the selected execution
profile and capacity/budget reservation IDs to an attempt and stores bounded
strategy/score evidence on its run. Assignment fields are all-null or all-set.
The Scheduler receives only the additional column reads/updates and outbox
insert access required by the transaction.

The transaction uses explicit tenant/run predicates and optimistic versions,
and clears its lease only with the state transition. Assignment, reservations,
daily-budget accounting, and the matching outbox envelope therefore commit or
roll back together.

Migration `000010_scheduler_policy_rejection` grants only the run failure and
completion columns plus attempt completion time needed to atomically finalize
a permanent scheduling policy rejection. The transaction completes the run as
`POLICY_REJECTED`, completes its current pending attempt as `CANCELLED`, and
inserts the failed-event outbox row. No schema object from an already committed
sub-phase is rewritten.

## Phase 6.10 handoff role

Migration `000011_agentrun_handoff` adds an immutable intent table keyed by
the scheduled event ID, with tenant/run/attempt/cluster identity and validated
JSON desired state. It grants the Scheduler narrow reads for the secure task
reference and attempt ceiling plus intent insert/replay access. The separate
`agentforge_handoff` role may read those intents and registered cluster/tenant
authorization and may read or insert durable processed-event markers; it
cannot update AgentRuns, attempts, reservations, or outbox rows.
