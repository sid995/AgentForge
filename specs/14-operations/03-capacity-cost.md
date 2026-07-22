# Capacity and Cost Operations

## Capacity signals

- queue depth and age.
- requested versus available CPU, memory, and GPU.
- pending Pod reasons.
- node-pool scale latency.
- model provider limits.
- build concurrency.

## Cost controls

- per-run estimate and hard ceiling.
- per-project and tenant budget.
- reservations before expensive work.
- model and infrastructure usage ledger.
- spot capacity for retryable workloads.
- artifact and image lifecycle policies.
- scale-to-zero for eligible applications.

Budget enforcement decisions are auditable and deterministic.

## Phase 5.3 implementation

Tenant scheduling policies store daily budget limits in integer minor units;
daily usage stores spent plus reserved amounts. Eligibility defers when no
balance remains and persists the evaluated usage with a stable reason. Phase
5.3 does not invent a per-run monetary estimate: Phase 5.6 adds the explicit
budget reservation attached to an attempt, and Phase 5.7 repeats admission in
the atomic scheduling transaction.

## Phase 5.5 deterministic selection

Scheduler selection uses integer basis-point utilization, reported queue depth,
registry cost attributes, and scheduling weight. It records score components
and applies stable cluster-ID tie-breaking. These registry signals guide a
deterministic choice but do not replace reservation checks or Kubernetes
admission.
