# Implementation Milestones

## M1: Durable command

Create project and run APIs, PostgreSQL migrations, domain state machine, idempotency, outbox, and tests.

## M2: Scheduling and CRD

Scheduler claims runs, selects a registered cluster, and commits immutable
scheduled intent with its outbox fact. A separate idempotent handoff creates or
exactly compares the AgentRun CR; the Scheduler process has no Kubernetes
client.

## M3: Operator vertical slice

Controller creates a restricted Job, the test runner completes, and Kubernetes
status reaches a terminal state with a validated artifact-manifest reference.
Projection into PostgreSQL and full artifact persistence belong to later
lifecycle and Runner work.

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
