# Phase 8 Plan: Artifact Service and Controlled Access

## Goal

Provide tenant-scoped durable artifact storage and controlled access to large
AgentRun outputs. PostgreSQL owns metadata and authorization records; object
storage owns payload bytes. Clients never receive bucket credentials.

## Governing requirements

- `15-llm-development/05-codex-execution-playbook.md` Phase 8
- `04-services/01-platform-api.md`, `05-apis/01-rest-api.md`
- `06-data/01-relational-schema.md`, `06-data/03-retention-backup-recovery.md`
- `09-security/02-identity-rbac-secrets.md`, `12-testing/01-test-strategy.md`

## Boundaries

- Phase 8 does not move or alter Phase 7's Runner-side artifact publication.
- Metadata contains tenant, project, run, attempt, object key, content type,
  size, checksum, retention class, and audited actor/time; payload bytes remain
  only in object storage.
- Every object key is constructed from trusted tenant/project/run/attempt IDs.
  Callers cannot submit a raw storage key or bucket credential.
- PostgreSQL repositories use transaction-local tenant context plus explicit
  tenant predicates and RLS. Cross-tenant access is an authorization failure.
- Presigned URLs are short lived, method- and key-bound, and issued only after
  the metadata authorization check. Multipart upload is deferred unless the
  metadata/API contract establishes a bounded need.

## Sub-phases

### 8.1 Storage contract and adapters

Add a Platform API `ArtifactStore` port, a deterministic tenant-scoped key
constructor, and filesystem plus MinIO/S3-compatible adapters. Cover put, get,
head, delete, checksum/metadata validation, and download presigning with a
single adapter conformance suite. Add an ADR for server-owned storage
configuration and key construction. No HTTP endpoint, database schema, or
client credential is introduced in this sub-phase.

**Validation status (2026-08-02):** Verified. Unit, race, and `go vet` checks
pass, and `make test-artifact-integration` passed against the local MinIO
service.

### 8.2 Metadata persistence and audit

Add a forward migration for tenant-owned artifact metadata and audit fields,
forced RLS, restrictive application-role grants, repository methods, and
PostgreSQL integration tests. The migration must be forward-safe and document
rollback limits. Metadata writes and any required integration event must commit
atomically.

**Validation status (2026-08-02):** Verified with immutable metadata,
tenant-scoped repository operations, forced RLS, and application-role
`SELECT`/`INSERT` grants. Domain, repository compilation, and static checks
pass locally. The PostgreSQL migration/RLS integration gate passed. No artifact metadata event
is currently defined by the approved event contract, so this sub-phase does
not introduce an outbox event.

### 8.3 Tenant-scoped metadata and access API

Add the versioned OpenAPI contract and authenticated Platform API handlers for
listing/getting run artifacts and issuing bounded download URLs. Enforce
server-derived tenant identity, run ownership, content/retention response
rules, stable error envelopes, and cross-tenant tests. Clients never submit raw
object keys or receive bucket credentials.

**Validation status (2026-08-02):** Verified with authenticated list/get/
download routes, explicit run-before-artifact authorization, safe metadata
responses, and a 1- to 900-second download lifetime. S3-compatible storage is
configured only through optional server-owned environment variables; without
it, metadata reads remain available and downloads fail closed with `503`. Unit,
HTTP, OpenAPI, and `go vet` checks pass locally. The MinIO and PostgreSQL
integration gates passed.

### 8.4 Retention and end-to-end validation

Add controlled deletion/retention transitions only after the metadata model is
validated. Run filesystem conformance, MinIO integration, PostgreSQL/RLS,
HTTP contract, and full repository gates. A completion audit may mark Phase 8
verified only after all required local and CI gates pass.

**Validation status (2026-08-02):** Verified with project-administrator-only
HOT payload deletion, object-first cleanup, retained immutable deletion
tombstones, and read filtering. Domain, application, HTTP/OpenAPI, repository
compilation, integration-tag compilation, race, and `go vet` checks pass
locally. Live MinIO and PostgreSQL integration, lint, and the full repository
validation gate passed.

## Acceptance evidence

- Durable objects are checksum verified through every supported adapter.
- Tenant/project/run-scoped keys cannot collide or escape their namespace.
- Metadata access and presigned downloads are tenant scoped and short lived.
- No client receives raw bucket credentials.
