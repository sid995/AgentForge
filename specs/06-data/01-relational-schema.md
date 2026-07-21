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
