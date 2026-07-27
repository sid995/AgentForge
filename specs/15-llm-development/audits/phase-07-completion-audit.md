# Phase 7 Completion Audit: Deterministic Agent Runner

**Audited:** 2026-07-27

## Scope reviewed

- ADR-005, the Phase 7 plan, Runner module/image, Operator runtime ConfigMap,
  security, test, local Compose, CI, status, and traceability updates
- signed task verification, deterministic template generation, heartbeats,
  confined command execution, cancellation, redaction, artifact publication,
  terminal evidence, and S3-compatible integration

## Acceptance evidence

| Criterion | Evidence | Result |
|---|---|---|
| Deterministic Runner can execute a controlled task | Go Runner module, signed Ed25519 task loader, controlled Python template, non-root image, and unit/race tests | Passed |
| Heartbeats and trajectory are visible | Ordered JSONL sink emits structured logs and durable trajectory chunks | Passed |
| Mandatory artifacts survive Pod deletion | Filesystem PVC scenario is checked into the real-kind gate; MinIO integration proves independent S3 durability | Pending pinned kind image |
| Cancellation terminates child processes | Executor applies process-group SIGTERM/SIGKILL and focused deadline tests | Passed |

## Security and contract review

- The task body remains outside the CR and immutable runtime ConfigMap. Only a
  mounted Secret may carry it; a non-secret projected trust bundle verifies its
  detached Ed25519 signature and run identity.
- Commands use a fixed executable allowlist and explicit argv. Relative
  working directories, symlink escapes, shell use, unsafe environment values,
  oversized output, and hung processes are rejected or bounded by tests.
- Artifact output is scoped by tenant/project/run/attempt. Mandatory payloads
  are written and verified before the final manifest; successful termination
  evidence is never emitted on partial publication.
- Phase 8 remains responsible for artifact metadata, tenant access APIs,
  presigned URLs, deletion, multipart policy, and retention. PostgreSQL
  lifecycle projection and real model/source integrations remain deferred.

## Validation record

| Command | Result |
|---|---|
| `make format` | Passed |
| `make lint` | Passed |
| `make test` | Passed |
| `go test -race ./agent-runner/...` | Passed |
| `go vet ./agent-runner/...` | Passed |
| `make test-runner-integration` | Passed against pinned MinIO |
| `make build-runner` | Passed; image runs as UID/GID 65532 |
| `docker compose config --quiet` | Passed |
| `make verify` and `git diff --check` | Passed |
| `make test-controller-kind` | Pending: pinned `kindest/node` image pull did not complete locally |

## Conclusion

Phase 7 implementation is complete and all non-kind gates pass. The phase is
not marked Verified because the real-Kubernetes Runner/PVC gate has not run in
this environment. Restore the pinned kind image and run
`make test-controller-kind`; then update the last traceability/status state to
Verified if it passes.
