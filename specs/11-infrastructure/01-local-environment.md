# Local Environment

## Components

- Docker Compose for PostgreSQL, Redis, Redpanda/Kafka, MinIO, and optional telemetry services.
- kind or k3d for Kubernetes.
- local registry.
- Argo CD.
- Prometheus, Grafana, Loki, Tempo, and OpenTelemetry Collector.

## Developer commands

Provide a Makefile or task runner for bootstrap, test, lint, local cluster creation, CRD installation, image loading, migrations, seed data, observability, and teardown.

## Reproducibility

Pin tool and image versions. A new developer should bootstrap from documented prerequisites without manually editing cluster resources.
