# Agent Runner

## Phase 7 protocol

The Phase 7 protocol is specified in
`15-llm-development/plans/phase-07-agent-runner.md`. It defines the versioned
runtime configuration, signed mounted-secret task envelope, trajectory and
heartbeat records, restricted command policy, artifact manifest, exit taxonomy,
and terminal-evidence ordering.

## Pipeline

1. Load signed task envelope.
2. Initialize workspace.
3. Retrieve source using scoped credentials.
4. Execute the selected agent implementation.
5. Enforce tool and command policies.
6. Record trajectory and heartbeats.
7. Run tests and validation.
8. Upload outputs.
9. Report a terminal result.

## Internal ports

- ModelClient.
- ToolExecutor.
- ArtifactStore.
- TrajectorySink.
- UsageMeter.
- PolicyEvaluator.
- CancellationSource.

## Safety

- Commands execute without shell interpolation unless explicitly required.
- Each command has timeout, output limit, environment allowlist, and working-directory constraint.
- Model output cannot directly bypass policy checks.
- The runner does not receive cloud or model-provider master credentials.

## Deterministic test mode

Provide a fake agent that generates known project templates and controlled failures. Integration tests must not require paid model calls.

## Phase 6.6 terminal evidence contract

Before exiting successfully, the runner writes a JSON object to
`/dev/termination-log` with exactly these fields:

```json
{"schemaVersion":1,"artifactManifestRef":"s3://example/manifests/run.json"}
```

The payload is bounded to 4096 bytes. `schemaVersion` must be `1`, and
`artifactManifestRef` must be a bounded URI-style secure reference. Exit code
zero without valid evidence is not platform success. The Operator parses only
this contract and never exposes arbitrary termination text.
