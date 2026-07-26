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

## Phase 6 AgentRun handoff telemetry

The handoff service exposes `/health/live`, PostgreSQL-backed
`/health/ready`, and Prometheus text at `/metrics`. Its single bounded
`agentforge_agentrun_handoff_events_total` counter uses only allowlisted
processed, duplicate, retry-attempt, dead-letter code, and routing-stage
dimensions; tenant, project, run, event, and cluster IDs are not metric labels.

Structured completion logs include event, tenant, project, run, attempt,
cluster, correlation, causation, and trace context as audit fields without raw
intent JSON, prompt/source content, credentials, or Kubernetes/database error
payloads. The deterministic AgentRun annotations retain bounded scheduled
event, correlation, causation, selected-cluster, and optional W3C trace
context for cross-process diagnosis.
