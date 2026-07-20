# Test Strategy

## Test pyramid

- Domain unit tests for state machines, policies, scheduling, cost, and validation.
- Adapter contract tests for PostgreSQL, Kafka, storage, providers, and Argo CD.
- Component tests with real local dependencies.
- Kubernetes controller tests with envtest and kind.
- End-to-end vertical-slice tests.
- Load, resilience, chaos, and security tests.

## Determinism

Tests use fake clock, deterministic IDs, seeded randomness, fake model provider, and controlled failure injection. Paid or nondeterministic model calls are excluded from required CI.

## Quality gates

Formatting, linting, static analysis, unit tests, race tests where relevant, migration verification, contract compatibility, image scan, and manifest validation.
