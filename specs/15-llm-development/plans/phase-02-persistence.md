# Phase 2 Plan: PostgreSQL Persistence and Tenant Isolation

## Status

Implemented and validated on 2026-07-21. This document uses the canonical repository
paths from `SPEC-MANIFEST.md`; the supplied Phase 2 prompt's older `05-data`
and `12-llm-development` paths map to `06-data` and `15-llm-development` in
this checkout.

## Scope

Phase 2 makes PostgreSQL authoritative for Tenant and Project persistence.
It delivers database bootstrap, migrations, connection pooling, accurate
readiness, tenant-scoped repositories, RLS, root Docker Compose support, and
one integration-test command. It excludes Redis, Kafka, Kubernetes, caching,
agent runs, and unauthenticated tenant-owned public APIs.

## Schema boundary

`tenants` contains a UUIDv7 identifier, immutable slug, display name, and UTC
timestamps. `projects` contains a UUIDv7 identifier, `tenant_id` foreign key,
validated name, repository URL, mutable aggregate version, and UTC timestamps.
The schema enforces tenant-prefixed uniqueness and the tenant/project history
index needed by later run queries. Every tenant-owned table includes
`tenant_id`; Phase 2's tenant-owned table is `projects`.

## Persistence choices

| Concern | Decision | Reasoning and consequence |
|---|---|---|
| Driver and pool | Go `database/sql` with the `pgx` driver | PostgreSQL support plus explicit maximum-open and maximum-idle pool controls, ping, and stats. It is a direct dependency of the Platform API. |
| Migration tool | `golang-migrate` with checked-in SQL | Explicit reviewed schema, RLS, and indexes; no generated migration drift. It adds a migration command but never runs at API startup. |
| Identifier generation | UUIDv7 in application code | Time-sortable globally unique IDs avoid database-specific ID generation and are testable. |
| Tenant context | `set_config('app.tenant_id', value, true)` in each repository transaction | `true` makes it transaction-local, preventing pooled-session leakage. Queries still carry an explicit tenant predicate. |
| Database roles | Separate migrator and `NOBYPASSRLS` app role | Limits application blast radius while allowing safe schema evolution. |
| Error translation | Repository maps PostgreSQL constraint and no-row outcomes to stable domain errors | API callers never receive raw SQL or connection details. |

## Connection and readiness model

`AGENTFORGE_DATABASE_URL` is required and must be a PostgreSQL URL. Pool
settings are bounded configuration: maximum connections, maximum idle
connections, connection lifetime, connection acquisition timeout, and startup
connect timeout. Startup validates connectivity before the listener becomes
ready. `/health/ready` pings the pool and returns a stable retryable dependency
error if the database is unavailable; `/health/live` remains process-only.
Pool statistics are exposed through a narrow metrics-hook interface for later
instrumentation without adding an observability SDK in this phase.

| Variable | Default | Validation |
|---|---:|---|
| `AGENTFORGE_DATABASE_URL` | none | PostgreSQL URL with host, user, and database |
| `AGENTFORGE_DATABASE_MAX_CONNS` | `10` | positive and bounded by application configuration |
| `AGENTFORGE_DATABASE_MAX_IDLE_CONNS` | `5` | non-negative and no greater than maximum connections |
| `AGENTFORGE_DATABASE_MAX_CONN_LIFETIME` | `30m` | positive duration |
| `AGENTFORGE_DATABASE_ACQUIRE_TIMEOUT` | `5s` | positive duration |
| `AGENTFORGE_DATABASE_CONNECT_TIMEOUT` | `5s` | positive duration |

## Repository and transaction boundary

Domain-specific `TenantRepository` and `ProjectRepository` ports support
tenant creation/seeding, project creation, scoped retrieval, and keyset project
listing. The PostgreSQL adapter begins one transaction per tenant-owned method,
sets transaction-local tenant context, executes only tenant-predicated SQL, and
commits or rolls back as one operation. There is no generic repository layer.

## Identity and HTTP boundary

The current Platform API has no authenticated tenant resolver. Adding a
client-supplied tenant header would violate `AGENTS.md`; therefore Phase 2 does
not expose public project endpoints despite the older prompt requesting them.
The use cases and repositories are ready for the Phase 3 identity abstraction.
This is a documented specification boundary, not an authorization bypass.

## Local and integration environment

The root `docker-compose.yml` starts PostgreSQL with an isolated local volume
and a health check. Future phases extend that file rather than introducing a
phase-specific Compose definition. `make test-integration` brings up a scoped
project from the root definition, applies migrations, runs tagged integration
tests using the application and migration URLs, and always tears the project
down. The command proves clean database creation, migration
ordering/repeatability, connection-pool behavior where practical, repository
isolation, RLS cross-tenant read/write rejection, direct-ID lookup resistance,
transaction reuse, and migration-role execution.

## Migration and recovery

See `06-data/04-migrations.md` and ADR-001. Migrations are ordered,
forward-only after release, and use expand-migrate-contract. Snapshots and PITR
must preserve migration versions. A production rollback may require a forward
corrective migration when data is transformed or removed.

## Acceptance evidence

- Migrations apply in order and are repeatable.
- PostgreSQL is the only authoritative store for implemented Tenant and Project
  entities.
- Projects are isolated by explicit repository predicates and RLS.
- Tenant context cannot leak across pooled transaction reuse.
- Readiness reflects live database connectivity.
- `make test-integration` runs the documented database test suite.

## Future limits

The initial single PostgreSQL cluster has no partitioning or tenant sharding.
High-volume append-only tables, read replicas, and connection proxies remain
future phases. RLS uses transaction-scoped context and therefore requires
repositories to preserve transaction boundaries when a pooler is introduced.
