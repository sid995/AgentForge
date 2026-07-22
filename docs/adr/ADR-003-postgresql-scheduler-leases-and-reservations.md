# ADR-003: PostgreSQL scheduler leases, policy locks, and reservations

- Status: Accepted
- Date: 2026-07-22
- Owners: Scheduler, Platform engineering

## Context

Multiple Scheduler replicas must claim tenant-owned runs fairly, enforce
concurrent limits, reserve shared cluster capacity, and recover after crashes.
PostgreSQL is already authoritative for run state and event intent. A separate
leader or Redis lock would create another correctness authority and would not
make quota, reservation, state, and outbox changes atomic.

## Decision

Scheduler replicas compete without leader election. Queue claims use short
PostgreSQL transactions with `FOR UPDATE SKIP LOCKED`, optimistic aggregate
versions, and expiring owner leases. Tenant policy rows serialize final quota
admission. Cluster capacity rows and unique run-attempt reservations serialize
capacity admission. Assignment, attempt/run transitions, capacity and budget
reservations, lease release, and the scheduled-event outbox row commit in one
transaction.

Kafka requested events are coalesced wake-up hints. Periodic PostgreSQL scans
remain authoritative. Redis is not used for leases, quotas, or reservations.
A dedicated least-privilege Scheduler database role may cross tenant boundaries
only for the explicitly granted queue, policy, registry, reservation, and
outbox operations. It cannot read prompt references, request hashes, secrets,
or unrelated tenant data.

## Alternatives considered

- One elected Scheduler leader: rejected because it adds failover machinery
  without removing the need for database concurrency controls.
- Kafka partition ownership as the queue: rejected because PostgreSQL state,
  cancellation, quotas, and reservations must remain authoritative.
- Redis leases and counters: rejected because expiry/counter state could diverge
  from transactional run state and outbox intent.
- Holding row locks during cluster calls: rejected because it lengthens
  transactions and amplifies contention; registry data is read locally and
  revalidated during the final transaction.

## Consequences

At-least-once wake-ups and expired-lease reclaims are normal. Final admission
must be re-evaluated under database locks even when a prior eligibility decision
was positive. Query plans and lock order are part of the contract. Scheduler
role grants, cross-tenant tests, lease recovery, reservation expiry, and
outbox-atomicity tests are mandatory.

## Revisit conditions

Revisit if PostgreSQL is no longer authoritative, measured contention cannot
meet the scheduling SLO, or a replacement can atomically preserve run state,
quotas, reservations, and event intent with equal isolation and recovery.
