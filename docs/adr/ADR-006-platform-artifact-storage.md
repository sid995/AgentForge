# ADR-006: Platform-owned artifact storage boundary

- Status: Accepted
- Date: 2026-08-02
- Owners: Platform API, Platform engineering, Security

## Context

Phase 7 makes Runner artifacts durable but intentionally leaves metadata,
controlled reads, deletion, and presigning to Phase 8. A Platform API service
needs to access artifacts across execution attempts without accepting a
caller-provided bucket, raw object key, or long-lived storage credential.

## Decision

The Platform API owns an `ArtifactStore` port with local filesystem and
S3-compatible adapters. Every operation accepts a typed tenant/project/run/
attempt key and derives the object name as
`tenants/<tenant>/projects/<project>/runs/<run>/attempts/<attempt>/<name>`.
The port verifies the producer-declared size and SHA-256 checksum before a put
is successful; the S3 adapter verifies the server-reported checksum after the
write.

Storage clients, buckets, roots, and credentials are injected only through
trusted server configuration. The filesystem adapter exists for deterministic
unit conformance tests and explicitly does not mint access URLs. The
S3-compatible adapter may issue only bounded download URLs; the metadata and
authorization layer that decides whether to issue one is introduced separately.

PostgreSQL remains authoritative for artifact metadata, retention, actor audit,
and tenant authorization. Object storage remains authoritative only for payload
bytes. No Phase 8.1 API or database migration accepts caller-provided object
keys or exposes bucket credentials to clients.

## Alternatives considered

- Reuse the Runner's write-only store from Platform API: rejected because it
  is a workload-local module with scoped credential loading and has no safe
  metadata/read/access boundary.
- Let HTTP clients select buckets or object keys: rejected because this would
  bypass tenant prefix construction and enable cross-tenant collisions.
- Fabricate filesystem presigned URLs: rejected because a `file://` path is not
  an access-controlled, expiring capability.

## Consequences

Phase 8.1 adds the MinIO Go client to the Platform API module, a filesystem
adapter, S3-compatible checksum verification, and a Docker-backed MinIO test
target. Metadata persistence, authorization, presigned-upload policy, and
retention/deletion APIs remain Phase 8.2 through 8.4 work.

## Validation

Filesystem adapter conformance is a unit gate. The S3-compatible conformance
suite runs through `make test-artifact-integration` against the pinned local
MinIO service and is included in CI.
