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
- Include migration application and repeated-execution evidence in Phase 2
  completion audits.

## Recovery implications

PostgreSQL snapshots and point-in-time recovery must include the migration
version table. Restore exercises apply only migrations newer than the restored
database version and document any expand-migrate-contract compatibility window.
