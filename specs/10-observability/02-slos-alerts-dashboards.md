# SLOs, Alerts, and Dashboards

## SLOs

- API successful request availability.
- Run acceptance latency.
- Queue scheduling latency.
- Workflow completion reliability excluding user-code failure.
- Deployment reconciliation latency.

## Critical alerts

- API error-budget fast burn.
- Database or Kafka unavailable.
- Scheduler makes no progress.
- Queue delay exceeds SLO.
- Controller reconciliation errors or workqueue growth.
- Cluster unschedulable capacity.
- Deployment failure spike.
- Unexpected cost spike.

## Dashboards

- Platform overview.
- API and dependency health.
- Scheduler and queues.
- Kubernetes execution and nodes.
- Agent quality and cost.
- Build and supply chain.
- Deployment and Argo CD health.
