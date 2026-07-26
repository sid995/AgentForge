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

## Phase 4.2 outbox relay isolation

AgentRun transactions write outbox rows as `agentforge_app`; a forced-RLS
insert policy requires the row tenant to match transaction-local
`app.tenant_id`. Cross-tenant publication runs under the distinct
`agentforge_relay` role. That role has `BYPASSRLS` because a relay must claim
events across tenants, but its grants are limited to `SELECT`, `UPDATE`, and
`DELETE` on `outbox_events`; it has no access to AgentRun, project, tenant, or
other business tables. Application and relay credentials must not be shared.

Outbox payload construction is allowlisted. The implemented run event includes
only identifiers, execution constraints, and a secure prompt reference;
it excludes raw prompt/source content, authorization material, provider keys,
and the internal idempotency request hash. Persisted failure reasons are
truncated to 1,000 characters and must not contain secrets.

Phase 4.4 consumer processing revalidates the complete envelope, headers,
topic, key, and serialized bytes before opening the tenant transaction. The
tenant comes from that validated envelope, not a business callback parameter.
Unknown errors become a generic stored reason. Malformed broker bytes and
unapproved headers are not copied into DLQ events; only a SHA-256 fingerprint,
byte count, bounded source coordinates, and trusted quarantine tenant are used.

## Phase 5.2 Scheduler isolation

Global fair claiming uses a distinct `agentforge_scheduler` login and database
role. It has `BYPASSRLS` only for queue coordination and receives column-level
grants for run identity, resource requests, runtime/profile, actor, priority,
version, and lease metadata plus current-attempt identity/status. It cannot
select prompt references, request hashes, cancellation reasons, project
repository URLs, or unrelated tenant tables. Application and Scheduler
credentials are not interchangeable.

Claim and renewal SQL always carry tenant identity forward and every targeted
renewal predicates both `tenant_id` and run ID. Integration tests prove a
foreign tenant renewal is not found and the Scheduler credential cannot read a
prompt reference. Logs replace raw database errors with a bounded category.

Phase 5.3 extends the Scheduler role only with tenant/project status, validated
policy and budget usage reads, a narrow policy-row lock permission, and
eligibility-decision inserts. Eligibility explanations are allowlisted and
bounded; they never include prompts, repository details, database errors, or
credentials. The trusted `created_by` value is used as the temporary user quota
key until the identity tables replace it; clients cannot supply it directly at
the Scheduler boundary.

Phases 5.4 through 5.8 extend that role incrementally with registry,
reservation, assignment, safe decision, and outbox columns only. The role can
insert events but cannot relay or delete them, and cannot read prompt/source or
authentication data. The non-root Scheduler image receives its isolated URL
through Compose/runtime configuration. Kafka is an optional hint channel; its
tenant and correlation metadata are accepted only after strict envelope,
header, topic, and key validation.

## Phase 6 Operator and handoff isolation

The scheduled-intent consumer uses the distinct `agentforge_handoff` database
role. It has `BYPASSRLS` only for its cross-tenant control-plane scope and may
read the cluster registry, cluster tenant allowlist, and immutable handoff
intents, then read/insert processed-event markers. It cannot mutate scheduling
intent, read AgentRun prompts or source, relay outbox rows, or use the
application/Scheduler credentials.

Kubernetes access uses an explicitly configured kubeconfig whose every cluster
ID maps to one known, unique context. The handoff identity must be limited to
creating/reading the deterministic tenant Namespace and AgentRun objects; it
cannot create Jobs, execution children, or write the AgentRun status
subresource. Unknown, duplicate, disabled, unauthorized, or mismatched cluster
intent is rejected before a Kubernetes write. Platform or cloud administrator
credentials are prohibited.

The Operator has the separate execution-controller identity. Referenced Secret
existence is checked through uncached `PartialObjectMetadata` reads with
get-only Secret RBAC, so secret payloads are neither read nor retained by the
controller cache. StorageClass inspection is also uncached and get-only. Only
the Operator owns execution resources and the AgentRun status subresource;
untrusted runner Pods receive no Kubernetes API token.

## Secrets

Store secrets in a cloud secret manager or Vault. Database rows hold references only. The agent accesses approved capabilities through brokers or scoped credentials. Every secret access is audited.
