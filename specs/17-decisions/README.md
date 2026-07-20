# Architecture Decision Records

Create ADRs using the template in `18-templates/adr-template.md`.

Initial decisions to record:

- Go as primary control-plane language.
- PostgreSQL as source of truth.
- Kafka with transactional outbox and idempotent consumers.
- Kubernetes Job plus AgentRun Operator.
- Separate build workloads using rootless BuildKit.
- GitOps deployment with Argo CD.
- Object storage for large artifacts.
- Hybrid current-state tables plus append-only audit and usage streams.
- One cloud provider first.
