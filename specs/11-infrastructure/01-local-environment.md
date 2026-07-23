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

Phase 4.3 implements PostgreSQL and pinned Redpanda in that one root Compose
file. `scripts/bootstrap-topics.sh` is the reproducible topic entry point; it
uses the `COMPOSE_PROJECT_NAME` value and fails when the broker is unavailable.
The broker integration gate uses a separate Compose project and host port so it
does not stop or mutate a developer's normal local stack.

Phase 6.10 adds the optional `agentrun-handoff` service to the same root
Compose file under the `kubernetes` profile. It mounts only the explicitly
configured kubeconfig, uses a declared cluster-ID-to-context map, and has CPU
and memory limits like every other Compose service. `.env.example` records all
Scheduler execution-intent, handoff, database-role, kubeconfig, port, and
resource-limit variables, including the bounded handoff processing timeout.
The configured kubeconfig context must use only the Namespace and AgentRun
permissions required by the handoff; administrator credentials are prohibited
outside disposable local development.
