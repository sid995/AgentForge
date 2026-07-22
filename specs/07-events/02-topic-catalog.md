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

`agent-run.scheduled.v1` is the Scheduler's durable assignment intent emitted
with the `PROVISIONING` transition, not a temporary claim notification.
`agent-run.provisioning-requested.v1` remains reserved and unimplemented until
the Phase 6 Operator input boundary is approved.

Phase 5.7 publishes executable v1 schemas, examples, compatibility baselines,
producer validation, and consumer fixtures for `agent-run.scheduled.v1` and
`agent-run.capacity-wait.v1`. Both are transactionally inserted with their
matching aggregate transition and contain only allowlisted assignment or safe
deferral metadata.

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

## Phase 4.3 local bootstrap

`scripts/bootstrap-topics.sh` creates these four lifecycle/governance topics
plus the AgentRun `1m`, `5m`, and `30m` retry topics and its DLQ. Local topics
use three partitions and one replica because the root Compose environment has
one broker. Retention is 7 days for run/build lifecycle, 14 days for deployment
and retry topics, 30 days for governance, and 90 days for the AgentRun DLQ.
Production replication and ACLs remain environment-owned configuration.
