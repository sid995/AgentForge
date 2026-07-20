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
