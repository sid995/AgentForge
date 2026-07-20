# Product Requirements

## Actors

- Developer: creates projects and agent runs.
- Project administrator: configures quotas, secrets, environments, and deployment rules.
- Platform operator: manages clusters, incidents, queues, and policies.
- Security auditor: reviews audit events, policies, and artifact provenance.

## Core use cases

1. Create and configure a project.
2. Submit an agent task with runtime and resource constraints.
3. Observe queueing, provisioning, execution, testing, building, and deployment.
4. Cancel or retry a run.
5. Inspect trajectory, logs, tests, artifacts, and cost.
6. Approve staging or production promotion.
7. Roll back a deployment.
8. Investigate an incident using linked telemetry and runbooks.

## Acceptance requirements

- Submission returns within 500 ms under normal load.
- Every accepted run has a durable identifier and status.
- Duplicate submission with the same idempotency key does not create another run.
- Agent workloads cannot access another tenant or platform credentials.
- Successful builds are addressed by immutable image digest.
- Production deployment requires explicit approval.
- Every privileged or state-changing action creates an audit event.
