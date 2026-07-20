# Load, Chaos, and Performance

## Load profiles

- Burst run submissions.
- Sustained queue growth.
- High-volume trajectory events.
- Concurrent dashboard polling or SSE clients.
- Build backlog and deployment waves.

## Chaos scenarios

- Kill API, scheduler, operator, and consumer replicas.
- PostgreSQL failover and connection exhaustion.
- Kafka unavailability and consumer lag.
- Redis loss.
- node eviction and OOMKilled workloads.
- object-storage timeout.
- registry and Argo CD failure.
- expired certificate and broken DNS.

## Outputs

Each exercise records hypothesis, load model, SLO, observed bottleneck, traces and metrics, remediation, and regression test.
