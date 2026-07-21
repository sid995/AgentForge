# Topic Catalog

The entries below are versioned event types, not physical Kafka topic names.
Physical topics group compatible event types by owned lifecycle stream as
defined in `07-events/04-event-architecture.md`.

## Run lifecycle

- `agent-run.requested.v1`
- `agent-run.scheduled.v1`
- `agent-run.capacity-wait.v1`
- `agent-run.provisioning-requested.v1`
- `agent-run.started.v1`
- `agent-run.execution-completed.v1`
- `agent-run.tests-accepted.v1`
- `agent-run.heartbeat.v1`
- `agent-run.completed.v1`
- `agent-run.failed.v1`
- `agent-run.timed-out.v1`
- `agent-run.cancel-requested.v1`
- `agent-run.cancelled.v1`
- `agent-run.retry-requested.v1`

Partition key: run ID.

Physical topic: `agentforge.agent-run.lifecycle.v1`.

## Build lifecycle

- `build.requested.v1`
- `build.started.v1`
- `build.completed.v1`
- `build.failed.v1`

Partition key: build ID or deterministic build identity.

Physical topic: `agentforge.build.lifecycle.v1`.

## Deployment lifecycle

- `deployment.requested.v1`
- `deployment.approved.v1`
- `deployment.status-changed.v1`
- `deployment.completed.v1`
- `deployment.failed.v1`

Partition key: project ID plus environment.

Physical topic: `agentforge.deployment.lifecycle.v1`.

## Governance

- `usage.recorded.v1`
- `audit.recorded.v1`
- `policy.decision.v1`
- `incident.created.v1`

Physical topic: `agentforge.governance.events.v1`, keyed by the owning
aggregate ID. Retry and dead-letter topics insert `.retry.<delay>` or `.dlq`
before the single version suffix, as specified in the event architecture.
