# CI/CD

## Pull request pipeline

1. Format, lint, and repository-control verification.
2. Unit and contract tests.
3. Go race and vet checks for relevant packages.
4. Event-schema example, immutable-major compatibility, producer, and supported
   consumer-fixture validation through `make verify-event-contracts`.
5. PostgreSQL migration and integration validation.
6. Kafka contract integration against the root-Compose Redpanda service.
7. Reproducible CRD/RBAC/deepcopy generation, Kubernetes 1.36 envtest, and the
   pinned kind 0.32.0/Kubernetes 1.36.1 lifecycle gate.
8. Build the non-root Platform API, Scheduler, AgentRun handoff, and Operator
   OCI images.
9. Generate SBOM, scan dependencies and images, and sign in a protected
   workflow when the release pipeline is implemented.

The compatibility baseline under `contracts/events/compatibility/` is retained
for the lifetime of a supported major. A pull request that removes an existing
field, changes its JSON type, constant, or reference, or changes the required
set fails the event-contract target. Optional properties may be added to the
live schema without modifying the baseline. Old supported consumer fixtures
remain under `contracts/events/fixtures/consumer/<major>/`.

## Promotion

Development may update GitOps automatically. Staging and production follow environment approval policy. Releases use immutable digests and record source commit, builder, tests, scan, and provenance.

## Rollback

Rollback is a new desired-state commit. Database migrations use expand-migrate-contract so application rollback remains possible.
