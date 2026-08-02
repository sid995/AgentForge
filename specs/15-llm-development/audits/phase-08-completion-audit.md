# Phase 8 Completion Audit: Artifact Service and Controlled Access

**Audited:** 2026-08-02
**Last reviewed:** 2026-08-02

## Scope reviewed

- ADR-006, Phase 8 plan, storage adapters, artifact metadata/tombstone
  migrations, application authorization, OpenAPI handlers, local configuration,
  test coverage, status, and traceability
- tenant-scoped object names, checksum metadata, presigned downloads,
  project-administrator-only deletion, and retained audit evidence

## Acceptance evidence

| Criterion | Evidence | Result |
|---|---|---|
| Objects are checksum verified and tenant scoped | Filesystem/S3 adapter conformance and typed tenant/project/run/attempt keys | Passed, including the live MinIO conformance gate |
| Metadata and downloads are tenant scoped | forced-RLS metadata repository, run-before-artifact application checks, authenticated list/get/download handlers, and HTTP tests | Passed, including PostgreSQL RLS integration |
| Presigned access is bounded | 1- to 900-second API lifetime and 15-minute port maximum | Passed locally |
| Controlled deletion preserves audit evidence | HOT-only administrator action, object-first delete, immutable tombstone, read filtering, and unit/HTTP/integration coverage | Passed, including PostgreSQL tombstone/RLS integration |
| Clients receive no storage credentials | server-only S3 configuration; no credential or raw-key metadata fields in the API contract | Passed locally |

## Security and lifecycle review

- Artifact records remain immutable. Deletion is represented by an append-only,
  tenant-scoped tombstone containing the actor, bounded reason, and UTC time.
- The object payload is removed before the tombstone is inserted. A retry after
  a crash treats a missing payload as reconciled and attempts to record the
  tombstone again.
- Only a `project-administrator` can request deletion, and only for HOT
  artifacts. ARCHIVE deletion, automatic expiry, and archive transitions remain
  deferred because the specifications do not establish an approved schedule or
  storage lifecycle contract.
- All artifact reads first establish caller-owned run scope. Cross-tenant and
  deleted artifacts return `404` without leaking metadata.

## Validation record

| Command | Result |
|---|---|
| focused domain/application/HTTP/config tests | Passed locally |
| focused `go test -race` and `go vet` | Passed locally |
| `make test-integration` | Passed, including migrations `000014` and `000015`, PostgreSQL RLS, and tombstone coverage |
| OpenAPI JSON validation and `make verify` | Passed locally |
| `make test` | Passed locally |
| `make test-artifact-integration` | Passed against local MinIO |
| `make lint` | Passed |
| `make check-all` | Passed: repository tools, formatting, lint, unit, event, PostgreSQL, Redpanda, Runner/MinIO, artifact/MinIO, controller, image-build, and verification gates |

## Conclusion

Phase 8 is Verified. The required local repository gates, including MinIO,
PostgreSQL migration/RLS/tombstone, Redpanda, Runner, controller, image-build,
and lint validation, passed on 2026-08-02. Automatic archive or expiry policy
is intentionally not claimed: it requires a separately approved retention
contract.
