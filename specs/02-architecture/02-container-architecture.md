# Container Architecture

## Control-plane containers

- `platform-api`: synchronous commands and normal queries.
- `scheduler`: claims queued runs and selects execution clusters.
- `workflow-orchestrator`: coordinates run, build, scan, and deployment saga.
- `model-gateway`: controlled model-provider proxy and usage meter.
- `build-service`: requests isolated image builds and records results.
- `deployment-service`: writes GitOps desired state and tracks Argo CD health.
- `usage-service`: append-only usage ledger and aggregates.
- `audit-service`: immutable audit event ingestion.
- `stream-gateway`: SSE status and log notifications.

## Data systems

- PostgreSQL: transactional state.
- Kafka: asynchronous event transport.
- Redis: cache, rate limit, leases, and ephemeral coordination.
- Object storage: artifacts and large logs.
- Registry: OCI images.

## Execution containers

- workspace initializer.
- agent runner.
- optional telemetry or egress proxy sidecar.
- isolated BuildKit worker.
