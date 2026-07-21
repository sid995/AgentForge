# Local Environment

## Components

- One root `docker-compose.yml` for PostgreSQL, Redis, Redpanda/Kafka, MinIO,
  and optional telemetry services. Implementation phases extend this file;
  phase-specific Compose files are not used.
- kind or k3d for Kubernetes.
- local registry.
- Argo CD.
- Prometheus, Grafana, Loki, Tempo, and OpenTelemetry Collector.

## Developer commands

Provide a Makefile or task runner for bootstrap, test, lint, local cluster creation, CRD installation, image loading, migrations, seed data, observability, and teardown.

## Reproducibility

Pin tool and image versions. A new developer should bootstrap from documented prerequisites without manually editing cluster resources.
Document every local environment variable in the checked-in `.env.example`;
developers copy it to ignored `.env` and replace development-only credentials
for any non-local environment.
