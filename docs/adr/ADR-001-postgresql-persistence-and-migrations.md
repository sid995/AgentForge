# ADR-001: PostgreSQL persistence and SQL migrations

- Status: Accepted
- Date: 2026-07-21
- Owners: Platform engineering

## Context

AgentForge requires an authoritative transactional store, tenant isolation at
both repository and database layers, and reproducible schema evolution. Phase
2 adds tenants and projects without introducing brokers, caches, or a new
service.

## Decision

Use PostgreSQL through Go's `database/sql` with the `pgx` driver for bounded
application connections and versioned SQL migrations executed by a checked-in Go migration command using
`golang-migrate`. Migrations run as `agentforge_migrator`; application traffic
runs as the least-privilege `agentforge_app` role with `NOBYPASSRLS`.

Tenant-owned repository operations run in explicit transactions, set
`app.tenant_id` with transaction-local `set_config`, and also include the
tenant predicate in every SQL statement. PostgreSQL RLS policies enforce the
same context as a second isolation boundary.

## Alternatives considered

- ORM-managed schema generation: rejected because reviewed SQL, RLS policies,
  indexes, and migration ordering must remain explicit and reproducible.
- Application predicates without RLS: rejected because a missed predicate
  would permit a cross-tenant read or write.
- RLS without repository predicates: rejected because privileged operational
  roles and future policy mistakes require defense in depth.
- One database role: rejected because the application must not gain schema or
  RLS-bypass privileges.

## Consequences

PostgreSQL and migration tooling become Phase 2 dependencies. Every
tenant-scoped query must retain an explicit tenant parameter and transaction
boundary; connection-pool callers cannot rely on session-level state. Database
schema evolution uses expand-migrate-contract and may require forward fixes
rather than a production down migration.

## Validation

Integration tests apply migrations twice, exercise repository isolation and
RLS as `agentforge_app`, verify tenant context is transaction-local, and prove
the migration role can still execute migrations. Readiness checks pool
connectivity rather than process state alone.

## Revisit conditions

Revisit only if PostgreSQL cannot meet the documented transactional, RLS, or
recovery requirements, or if a migration tool cannot maintain reproducible,
checked-in SQL history.
