# Scope and Releases

## Release 0: Local vertical slice

- Go control-plane API.
- PostgreSQL source of truth.
- Transactional outbox.
- Scheduler loop.
- Fake deterministic agent runner.
- kind or k3d cluster.
- AgentRun CRD and controller.
- Basic metrics and logs.

## Release 1: Durable execution

- Kafka or Redpanda.
- Real asynchronous consumers.
- Retry policies and dead-letter handling.
- MinIO artifact storage.
- Workspace lifecycle.
- Cancellation and timeout handling.

## Release 2: Build and GitOps

- Rootless BuildKit build jobs.
- Registry publishing by digest.
- SBOM and vulnerability scan.
- GitOps environment repository.
- Argo CD deployment and rollback.

## Release 3: Security and operations

- OIDC and RBAC.
- Tenant isolation.
- Network policies and sandbox profiles.
- Prometheus, Grafana, Loki, Tempo, Alertmanager.
- Runbooks and incident simulations.

## Release 4: Cloud and scale

- Terraform-provisioned GKE or EKS.
- Managed database, cache, storage, and registry.
- Autoscaling and multi-cluster scheduling.
- Cloudflare edge integration.
