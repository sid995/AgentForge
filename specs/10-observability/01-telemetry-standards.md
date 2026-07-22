# Telemetry Standards

## Logs

Structured JSON with timestamp, level, service, environment, cluster, tenant, project, run, request, and trace identifiers. Do not use high-cardinality values as Loki labels.

## Metrics

- RED metrics for services.
- USE metrics for infrastructure.
- Workflow metrics for queue delay, run duration, attempts, success, build, deployment, tokens, and cost.
- Bounded label cardinality.

## Traces

OpenTelemetry propagation across HTTP, Kafka headers, controller actions, model calls, build, and deployment. Sample intelligently and retain error traces at a higher rate.

## Correlation

Every API command receives a request ID and trace ID. Events preserve correlation and causation. Kubernetes resources include run identifiers as labels and trace identifiers as annotations where safe.

## Phase 5 Scheduler telemetry

The Scheduler exposes `/health/live`, PostgreSQL/schema-backed
`/health/ready`, and Prometheus text at `/metrics`. Metrics use bounded
strategy/outcome/reason dimensions and never tenant or run IDs. Structured
decision logs may contain tenant/run correlation fields but never prompts,
source, credentials, authorization, or raw database/broker errors. Kafka wake
hints retain correlation and `traceparent`; authoritative polling uses the run
ID as correlation when no request trace exists.

## Phase 6 Operator telemetry

AgentRun reconciliation logs attach tenant, project, run, and attempt values
as structured correlation fields. These values are not Prometheus labels.
Operator metrics expose bounded reconciliation outcomes, duration, and status
write outcomes (`updated`, `unchanged`, `conflict`, or `error`). Metrics use the
controller-runtime Prometheus registry; `prometheus/client_golang` is a direct
dependency only to define these collectors and adds no separate server or
runtime process.
