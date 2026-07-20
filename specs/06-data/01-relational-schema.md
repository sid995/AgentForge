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
