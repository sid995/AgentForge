# Scheduler Service

## Responsibilities

- Claim eligible queued runs.
- Enforce tenant concurrency, budget, and cluster policy.
- Rank tasks fairly.
- Select an execution cluster and workload profile.
- Create deterministic AgentRun custom resources.
- Repair partially completed scheduling operations.

## Scheduling strategies

- Weighted fair scheduling by tenant.
- Priority plus queue-age scoring.
- Region and runtime affinity.
- Least-loaded or lowest-cost cluster choice.

## Concurrency safety

Use `FOR UPDATE SKIP LOCKED` or equivalent durable claim logic. Store lease owner and expiry. Creation of a CR uses a deterministic name based on run and attempt.

Phase 5 uses competing PostgreSQL claimers rather than leader election. The
complete process, fairness, lease, quota, registry, selection, reservation,
security, observability, and test design is recorded in
`15-llm-development/plans/phase-05-scheduler.md` and ADR-003. Kafka request
events are wake-up hints; periodic PostgreSQL scans remain authoritative.

Claiming a run is internal coordination and does not emit a scheduled fact.
`agent-run.scheduled.v1` is written only when cluster selection, reservations,
attempt assignment, and the `PROVISIONING` transition commit atomically.

## Backpressure

When execution capacity is exhausted, retain runs in `CAPACITY_WAIT`, expose queue delay, and reject new low-priority work only when configured thresholds are exceeded.

Capacity-related, concurrency, and budget decisions are temporary deferrals.
Tenant/project suspension and unsupported runtime/profile decisions are
permanent scheduling rejections. Storage quota is deferred until runs have an
approved storage-request contract.
