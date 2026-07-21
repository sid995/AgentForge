# Identity, RBAC, and Secrets

## Human identity

OIDC authentication with short-lived tokens. Membership maps users to tenant and project roles.

## Workload identity

Services and Jobs use cloud workload identity or short-lived internal credentials. Static cloud keys are forbidden.

## Authorization

Enforce permission checks at API, service, repository, RLS, Kubernetes RBAC, object-storage prefix, and consumer boundaries.

## Phase 2 PostgreSQL tenant isolation

The `agentforge_app` database role is `NOBYPASSRLS` and has only the DML grants
required for implemented project repositories. `agentforge_migrator` is the
separate privileged role that owns schema changes and migrations. A repository
begins a transaction, executes `set_config('app.tenant_id', tenantID, true)`,
then uses an explicit `tenant_id` predicate in every project query. The third
argument makes the setting transaction-local, so commit or rollback clears it
before a pooled connection is reused. RLS policies enforce the same tenant ID
for reads and writes; neither layer is treated as sufficient on its own.

## Secrets

Store secrets in a cloud secret manager or Vault. Database rows hold references only. The agent accesses approved capabilities through brokers or scoped credentials. Every secret access is audited.
