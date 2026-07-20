# Implementation Milestones

## M1: Durable command

Create project and run APIs, PostgreSQL migrations, domain state machine, idempotency, outbox, and tests.

## M2: Scheduling and CRD

Scheduler claims runs, selects local cluster, creates AgentRun CR, and repairs partial state.

## M3: Operator vertical slice

Controller creates restricted Job, fake runner completes, status reaches terminal state, artifacts are recorded.

## M4: Kafka workflow

Outbox relay, versioned events, idempotent consumers, retries, and DLQ.

## M5: Build and release

Source artifact, rootless image build, scan, SBOM, signature, immutable digest.

## M6: GitOps deployment

Environment repository, Argo CD, status tracking, approvals, and rollback.

## M7: Observability and incidents

Telemetry, SLOs, alerts, dashboards, runbooks, and chaos exercises.

## M8: Identity, isolation, and cloud

OIDC, RBAC, RLS, secret broker, network policy, Terraform cloud environment, and autoscaling.
