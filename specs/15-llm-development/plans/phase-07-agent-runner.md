# Phase 7 Plan: Deterministic Agent Runner

## Scope and ownership

Phase 7 adds the isolated workload executable that the Phase 6 Operator already
creates. It is a separate Go module because it has a distinct runtime image,
untrusted-workload boundary, dependencies, and test lifecycle. It does not add
a control-plane service.

The runner owns task verification, deterministic workspace generation,
restricted test execution, trajectory and heartbeat records, mandatory
artifact publication, and terminal evidence. The Operator continues to own
Kubernetes resources and status. PostgreSQL lifecycle projection, trajectory
and artifact APIs, real model access, source retrieval, and general artifact
storage capabilities are deferred to later phases.

Each numbered sub-phase is implemented, reviewed, validated, and committed
before the next begins.

## Versioned runner contracts

### Runtime configuration v1

The Operator-owned immutable runtime ConfigMap contains `config.json` with
`schemaVersion: 1`, the existing run/attempt identity and references, and
`timeoutSeconds`. It contains no task body, credentials, prompts, source, or
artifact credentials. The runner reads its path from `AGENTFORGE_RUN_CONFIG`.

### Signed task envelope v1

`taskRef` is `secret://<secret-name>/<key>`. The named Secret must be among the
already projected `secretRefs`; its sibling `<key>.sig` contains a detached
base64 Ed25519 signature over the exact envelope bytes. The envelope supplies
its `schemaVersion`, `keyId`, tenant/project/run/attempt identity, controlled
template name, and tokenized test commands. The runner loads public keys only
from a non-secret `task-trust.json` in exactly one projected configuration
reference. It rejects malformed, unsigned, unknown-key, bad-signature, and
identity-mismatched envelopes without logging their contents.

Phase 7 supports only the signed, mounted-secret task source. Vault, source
repository retrieval, and model-driven task construction remain later
capabilities.

### Execution and trajectory v1

The deterministic agent can generate only the checked-in Python template and
uses explicit command argument vectors. A command runs below `/workspace`, with
canonical-path and symlink confinement, a trusted executable allowlist, a
minimal environment, bounded timeout/output, and process-group cleanup.

Trajectory is ordered JSON Lines. Every record has `schemaVersion`, a
monotonic sequence number, timestamp, type, and a bounded safe payload.
Heartbeats are trajectory records and structured logs emitted every five
seconds. Secret values and task bodies are redacted before either sink.

### Artifacts and terminal evidence v1

`/etc/agentforge/artifacts/config.json` selects `filesystem` or `s3` storage.
The filesystem root must remain below `/workspace`; S3 credentials are read
only from an already projected Secret. The runner writes all keys under a
deterministic tenant/project/run/attempt prefix and records SHA-256, size, and
media type in the final manifest.

Source snapshot, test report, trajectory chunks, and bounded stdout/stderr are
published and verified before the final result manifest. The final manifest is
the commit point: the runner writes the existing strict termination evidence
only after it is durable. A cancellation, timeout, command failure, or failed
artifact upload exits non-zero and cannot report successful evidence.

## Exit taxonomy

| Code | Meaning |
|---|---|
| 0 | Successful execution with durable mandatory artifacts and valid termination evidence |
| 2 | Invalid runtime configuration or task envelope |
| 3 | Policy or command confinement rejection |
| 4 | Controlled command or test failure |
| 5 | Runner deadline exceeded |
| 6 | SIGTERM cancellation completed |
| 7 | Mandatory artifact publication failed |
| 8 | Unexpected internal failure |

## Acceptance and validation

- A real AgentRun Job runs the non-root deterministic image and produces valid
  terminal evidence.
- Container logs and trajectory chunks show ordered heartbeats without secrets.
- The filesystem adapter proves mandatory artifacts remain on the workspace PVC
  after Pod deletion; a MinIO integration proves S3-compatible publication.
- SIGTERM and deadline cancellation terminate the command process group.
- Unit, adversarial, container, MinIO, Operator envtest, and kind coverage
  prove the contracts above. The phase audit records every passed gate and all
  intentionally deferred lifecycle/API work.
