# PostgreSQL Migration Contract

## Scope

Phase 2 establishes PostgreSQL as the authoritative store for implemented
entities. Schema changes use checked-in, forward-only SQL migrations in
`db/migrations/`; the migration runner records applied versions in PostgreSQL.

Phase 3.2 adds `000002_agent_runs_and_attempts`. It first adds the project
`(id, tenant_id)` ownership key needed by the composite foreign key, then
creates run and attempt constraints, idempotency uniqueness, history and
active-work indexes, least-privilege application grants, and forced RLS. Its
down migration removes the dependent tables before the temporary composite
project key.

Phase 4.2 adds `000003_transactional_outbox`. It creates the outbox table,
claim and retention indexes, constrained publication state, an application
insert policy, and narrowly scoped relay grants. Its down migration drops the
table and its dependent policies/indexes. It deliberately retains the relay
role because role removal is unsafe when another database in the PostgreSQL
cluster may grant or own objects through that role.

Phase 4.4 adds `000004_processed_events`. It creates the consumer/event primary
key, source and aggregate metadata constraints, tenant/time index,
least-privilege application grants, and forced tenant RLS. Its local down
migration drops the marker table; production rollback retains data and uses a
corrective forward migration under the normal migration contract.

## Naming and ordering

Migration files use a six-digit, increasing version and a kebab-case purpose:

```text
000001_tenants_and_projects.up.sql
000001_tenants_and_projects.down.sql
```

Each version has exactly one `up` and one `down` migration. A released
migration is immutable. Corrections are new forward migrations; they must not
rewrite applied history.

## Roles and execution

- `agentforge_migrator` owns schema changes and is the only role allowed to run
  migrations.
- `agentforge_app` is a least-privilege, `NOBYPASSRLS` application role.
- `agentforge_relay` is an isolated cross-tenant relay role with `BYPASSRLS`
  and DML access only to `outbox_events`.
- The migration role can create and alter roles, tables, indexes, and RLS
  policies. The application role has only the schema and DML privileges needed
  by implemented repositories.

The migration runner must connect with the migration role. Application startup
never attempts a schema change.

## Compatibility and rollback

Migrations follow expand-migrate-contract. Destructive changes require a
separate release after all readers and writers no longer depend on the old
shape. A down migration is provided for local development before a release,
but production rollback is an application rollback plus a corrective forward
migration when data loss or incompatible data transformations are possible.

## Validation

- Apply all migrations to an empty database.
- Re-run application with no pending migrations and verify `no change`.
- Verify version ordering and paired files in repository validation.
- Run migrations with the privileged role and integration tests with the
  application role.
- Include migration application and repeated-execution evidence in the
  governing phase completion audit.

## Recovery implications

PostgreSQL snapshots and point-in-time recovery must include the migration
version table. Restore exercises apply only migrations newer than the restored
database version and document any expand-migrate-contract compatibility window.
