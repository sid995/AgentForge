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

## Backpressure

When execution capacity is exhausted, retain runs in `CAPACITY_WAIT`, expose queue delay, and reject new low-priority work only when configured thresholds are exceeded.
