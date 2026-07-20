# Design Pattern Map

| Pattern | Applied location | Required behavior |
|---|---|---|
| Hexagonal architecture | All Go services | Domain depends on ports, not SDKs |
| State machine | Runs, attempts, builds, deployments | Reject invalid transitions |
| Transactional outbox | Services changing DB and publishing events | Atomic local state and event intent |
| Idempotent consumer | Every Kafka consumer | Duplicate event produces no duplicate side effect |
| Saga orchestration | End-to-end workflow | Explicit progress and compensations |
| Reconciliation loop | Kubernetes operator and Argo CD | Crash-safe convergence |
| Strategy | Scheduling, model routing, retry, deployment | Algorithms selected by policy/configuration |
| Adapter | Providers and external APIs | External vocabulary does not leak into domain |
| Circuit breaker | Model, Git, storage, registry APIs | Prevent repeated calls during dependency failure |
| Bulkhead | Tenant quotas, queues, node pools | One failure domain cannot consume all capacity |
| CQRS | Commands and dashboard queries | Write correctness and read efficiency separated |
| Cache-aside | Read-heavy status and configuration | PostgreSQL remains authoritative |
