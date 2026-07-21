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
metadata, and UTC lifecycle timestamps. The composite project/tenant foreign
key prevents a run from referencing a project in another tenant.

An attempt stores its owning run and tenant, immutable monotonic attempt number,
lifecycle status, normalized failure category, optimistic version, opaque
cluster/workload references, and UTC timestamps. `(run_id, attempt_number)` is
unique. `agent_runs` has tenant/project history and partial scheduler-queue
indexes; attempts have a partial active-attempt index. Both tables have forced
RLS for `agentforge_app`, transaction-local tenant context, explicit repository
tenant predicates, and only `SELECT`, `INSERT`, and `UPDATE` grants.
