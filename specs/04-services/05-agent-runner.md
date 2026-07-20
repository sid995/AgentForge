# Agent Runner

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
