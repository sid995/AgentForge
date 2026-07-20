# CI/CD

## Pull request pipeline

1. Format and lint.
2. Unit and contract tests.
3. Go race tests for relevant packages.
4. Database migration validation.
5. CRD and manifest schema validation.
6. Build OCI image.
7. Generate SBOM.
8. Scan dependencies and image.
9. Sign image in protected workflow.

## Promotion

Development may update GitOps automatically. Staging and production follow environment approval policy. Releases use immutable digests and record source commit, builder, tests, scan, and provenance.

## Rollback

Rollback is a new desired-state commit. Database migrations use expand-migrate-contract so application rollback remains possible.
