# Phase 7 Completion Audit: Deterministic Agent Runner

**Audited:** 2026-07-27
**Last reviewed:** 2026-08-02

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
| Mandatory artifacts survive Pod deletion | Filesystem Runner/PVC scenario is checked into the kind gate; MinIO integration independently proves S3 durability | Passed locally; the CI kind job is re-enabled and awaiting its first post-fix result |
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
| `bash -n operator/test/kind/lifecycle.sh` | Passed |
| `make test-controller-kind` | Passed locally on 2026-08-02: real Runner/PVC execution, retained-PVC recovery, missing-evidence handling, cleanup, and transient retry |
| GitHub Actions kind job | Re-enabled in `.github/workflows/repository-checks.yml`; CI result pending |

## Conclusion

Phase 7 implementation and local validation, including the real Kubernetes
Runner/PVC gate, are complete. The kind workflow job is now enabled and its
first post-fix CI result is pending; Phase 7 remains unmarked as Verified until
that CI run passes. The lifecycle script now fails fast on terminal Runner
failure and avoids SIGPIPE-prone early pipeline termination while resolving
the digest-qualified image.
