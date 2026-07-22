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

## Phase 5.2 implementation

The PostgreSQL adapter claims bounded fairness rounds with `FOR UPDATE SKIP
LOCKED`. It orders each tenant's candidates by effective priority descending,
then creation time and stable ID; tenant row numbers interleave first candidates
before one tenant receives a second claim. Queue age supplies a bounded aging
boost. Eligible states are `QUEUED`, due `CAPACITY_WAIT`, and expired
`SCHEDULING`, always with a current `PENDING` attempt.

Claims atomically set status, owner, expiry, update time, and aggregate version.
Active leases are not returned; expired leases are reclaimable. Renewal needs
tenant ID, run ID, owner, an unexpired lease, and the expected version. The
application claimer emits bounded result/queue-age metrics and structured logs
without raw database errors. No quota or cluster decision exists in this
sub-phase.

## Phase 5.3 implementation

Focused domain rules return `ELIGIBLE`, temporary `DEFER`, or permanent
`REJECT` with a stable code and bounded explanation. Tenant/project suspension
and unsupported runtime/profile reject. Tenant/user concurrency, queue excess,
CPU, memory, missing policy, and exhausted daily budget defer with a bounded
next-eligibility time.

The PostgreSQL adapter locks the tenant policy row, reads tenant/project status,
computes active/waiting counts and resource totals with indexed aggregate SQL,
reads current-day budget usage, evaluates the domain policy, and inserts an
append-only decision in one transaction. Resource equality is allowed; a limit
is blocked only when the candidate would exceed it. Queue equality is allowed
because the claimed candidate is already included in the waiting count. Final
quota admission is re-evaluated with reservations in Phase 5.7.

## Phase 5.4 implementation

The global execution-cluster registry stores validated region, status,
maintenance, runtime/profile support, integer cost/weight, heartbeat, and
optimistic version metadata. Append-only capacity snapshots report allocatable
CPU/memory and queued workloads. Restricted clusters require an explicit
tenant allowlist; unrestricted clusters are available to every tenant.

The Scheduler-only repository supports registration, version-checked updates,
latest-snapshot reads, and deterministic candidate listing. Candidates must be
active, outside maintenance, heartbeat- and capacity-fresh, runtime/profile
compatible, and tenant-allowed. The internal `clusterctl` command uses the
same validation and repository path; it is not a tenant-facing API.

## Phase 5.5 implementation

The replaceable `Strategy` boundary provides `least-loaded` and
`region-affinity` selection. Both exclude insufficient capacity and compute an
integer score from projected CPU/memory utilization, reported queued work,
approved integer cost, and scheduling weight. Ties compare utilization, queue,
cost, then cluster ID. Region affinity first ranks sufficient preferred-region
candidates and deterministically falls back to the global set.

No sufficient candidate returns a typed temporary deferral. Each successful
decision records the strategy, cluster, reason, score components, fallback,
and time. The observable application wrapper logs only safe decision fields
and uses bounded strategy/result metric dimensions. The strategy is selected
by the validated `AGENTFORGE_SCHEDULER_STRATEGY` configuration value.

## Phase 5.6 implementation

Capacity and budget reservations are durable, tenant/run/attempt-attached
records created in one PostgreSQL transaction. Admission locks the tenant
policy and cluster in stable order, subtracts active unexpired reservations
from the latest reported capacity, locks daily budget usage, and rejects
overbooking with typed temporary capacity or budget errors. A unique active
run-attempt reservation plus exact input comparison makes retries idempotent.

Release, execution-start settlement, and expired reclaim are conditional and
idempotent. Budget settlement moves reserved integer minor units to spent;
release and expiry return them to availability. Scheduler reservations remain
estimates and do not replace Kubernetes ResourceQuota, admission, or observed
node capacity. Phase 5.7 composes this admission into the run transition and
outbox transaction.

## Phase 5.7 implementation

The scheduling-intent transaction locks and revalidates the owned run and
pending attempt, tenant policy and statuses, cluster health/support/allowlist,
capacity reservations, and UTC-day budget usage. It creates both reservations,
assigns the attempt as `STARTING`, moves the run to `PROVISIONING`, clears the
lease, records the strategy/score/reason, and inserts
`agent-run.scheduled.v1` into the outbox before one commit.

Exact replay returns the original assignment, reservation IDs, aggregate
versions, and event ID. Any changed cluster, profile, resource, budget, TTL,
strategy, or score conflicts. No-capacity handling independently moves an
owned run to `CAPACITY_WAIT`, records a bounded next-eligible decision, clears
the lease, and inserts `agent-run.capacity-wait.v1` atomically. Neither path
creates a Kubernetes resource.
