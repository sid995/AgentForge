# Phase 5 Completion Audit: Scheduler

**Audited:** 2026-07-22

## Scope reviewed

- Phase 5 plan, ADR-003, Scheduler/state-machine/data/event/security,
  observability, capacity/cost, and testing specifications
- Queue claims and lease renewal, eligibility and quota policy, cluster
  registry, deterministic strategies, capacity/budget reservations, atomic
  scheduling intent, event contracts, process lifecycle, root Compose, and
  container build
- Unit, PostgreSQL concurrency, service, event compatibility, race, static,
  repository, Compose, and image gates

The supplied prompt used the obsolete `12-llm-development` audit prefix. The
canonical manifest path is `15-llm-development/audits/`.

## Phase gate evidence

| Requirement | Evidence | Result |
|---|---|---|
| Multiple instances cannot schedule one attempt twice | Fair claims use row locks and leases; the final transaction locks the run/attempt and exact concurrent replay returns the same reservation and event IDs | Passed |
| Expired leases recover | Queue tests prove active leases cannot be stolen and expired `SCHEDULING` rows are reclaimed with a new owner/version | Passed |
| Quotas are enforced without noisy-neighbour blocking | Focused rules, indexed aggregates, policy-row locking, and final transactional revalidation cover tenant/user concurrency, queue, CPU, memory, budget, runtime/profile, and suspension; another tenant remains eligible under concurrent saturation | Passed |
| Cluster selection is safe and replaceable | Registry freshness, maintenance, runtime/profile, and tenant allowlist filters precede replaceable least-loaded/region-affinity strategies with integer scores and stable ID ties | Passed |
| Reservations are bounded and expirable | Policy/cluster/daily-usage locks prevent capacity or budget overbooking; unique active-attempt indexes, release, settle, and bounded expiry reclaim are idempotent | Passed |
| Scheduling state and events are atomic | Assignment, reservations, daily budget, attempt `STARTING`, run `PROVISIONING`, and `agent-run.scheduled.v1` commit in one transaction; no-capacity and policy-rejection transitions also include their outbox facts | Passed |
| Process is bounded and observable | Fixed workers, bounded channel, available-slot claims, jittered scan backoff, optional coalesced Kafka hints, active lease renewal, graceful cancellation, health/readiness, and bounded-cardinality metrics are implemented | Passed |
| Documentation and traceability are current | Status, traceability, component/data/event/observability/testing/cost specs, Phase plan, README, `.env.example`, nested instructions, and this audit point to code/test evidence | Passed |

## Review correction

Final review found that permanent scheduling-admission rejection had an
implementation path but no authoritative `SCHEDULING -> POLICY_REJECTED`
transition. The state-machine contract and domain guard now include that
transition. Its transaction cancels and completes the pending attempt,
terminalizes the run with a policy failure, and inserts the single
`agent-run.failed.v1` outbox fact. Integration coverage proves exact replay
returns the same run, attempt, and event versions.

## Concrete distributed-systems interleavings

### Duplicate claim and finalization

Interleaving: schedulers A and B scan the same queued run. A locks it with
`FOR UPDATE SKIP LOCKED`; B skips it. A commits `SCHEDULING`, owner A, expiry,
and version N+1. B cannot claim until expiry. If two workers nevertheless call
the finalizer with the same claim, the first locks the run, commits version
N+2 and clears the lease. The second then observes `PROVISIONING` and returns
the existing assignment only if cluster, profile, resource, budget, TTL,
strategy, and score match. Changed input conflicts. Database uniqueness also
permits only one active run-attempt reservation.

### Lease expiry during active work

Interleaving: A evaluates policy, then renews its still-owned lease using the
expected version before cluster/final intent work. If renewal loses to expiry
or a competing owner, A stops. If A dies after renewal, B can reclaim after the
new expiry. If A's final transaction and B's reclaim race, the run row lock and
version/owner predicates select one winner; the loser cannot create a second
reservation or event.

### Concurrent quota and capacity admission

Interleaving: two runs for one tenant pass a preliminary policy read. Final
transactions serialize on the tenant policy, re-read active usage, then lock
the chosen cluster and daily budget. The second sees the first committed
reservation/state and defers when a limit would be exceeded. Runs from another
tenant use a different policy lock, so a saturated tenant cannot monopolize
all admission transactions. Runs for different tenants choosing one cluster
still serialize on that cluster to prevent physical over-reservation.

### Crash boundaries and reservation leakage

Interleaving: the process dies before the scheduling transaction commits. Run,
attempt, reservations, budget delta, and outbox row all roll back; the lease
eventually expires. It dies after commit but before returning: replay reads the
same durable reservation and event IDs. It dies after a standalone reservation
but before explicit release: the expiry reclaimer changes both ledgers and
returns reserved budget. Settled reservations are not reclaimed as pending.

### Broker loss and poison wake hints

Kafka is never queue authority. Broker loss only delays a wake; jittered
PostgreSQL scans continue and readiness depends on PostgreSQL/schema, not the
broker. Duplicate requested hints coalesce into a one-slot wake channel.
Malformed optional hints are safely acknowledged without business effect so
they cannot pin the hint group; authoritative database state remains intact.

### Tenant and sensitive-data isolation

The cross-tenant Scheduler role uses `BYPASSRLS` only for its control-plane
scope and receives column/table grants incrementally. Claim tests prove it
cannot read prompt references. Every mutation after a global claim predicates
tenant and run identity. Logs suppress raw adapter/broker errors and never log
prompts, source, authorization, or credentials; metric labels exclude tenant
and run IDs.

## Query and transaction review

- Claims are bounded, index-supported, and commit before policy/cluster work.
- Policy aggregates use partial indexes and do not load tenant run collections.
- Registry reads use latest-snapshot indexes and deterministic ordering.
- Reservation totals use active partial indexes; reclaim is bounded.
- Final intent performs no broker or Kubernetes call while holding locks.
- Lock order is run, attempt, tenant policy, cluster, daily budget, then
  reservation/outbox inserts. Standalone reservation transitions lock policy
  before reservation rows, avoiding a reverse cluster/policy cycle.

## Validation record

The final command results below were run after all Phase 5 code and
documentation changes.

| Command | Result |
|---|---|
| `make check-tools` | Passed; required tools present, optional later-phase tools reported honestly |
| `make format` | Passed |
| `make lint` | Passed with `golangci-lint` v2.9.0 container |
| `make test` | Passed |
| `make verify-event-contracts` | Passed for schemas, examples, immutable baselines, producers, and consumer fixtures |
| `go test -race ./services/platform-api/...` | Passed |
| `go vet ./services/platform-api/...` | Passed |
| `make verify` | Passed |
| `docker compose config -q` | Passed; single root Compose only |
| `make test-integration` | Passed against fresh PostgreSQL migrations 1 through 10 |
| `make test-events-integration` | Passed against isolated pinned Redpanda and bootstrapped topics |
| `make build-platform-api` | Passed |
| `make build-scheduler` | Passed with the non-root distroless image |
| Root Compose Scheduler smoke | Passed: PostgreSQL, migrations, Scheduler start, `/health/live`, `/health/ready`, and `/metrics` |
| `make test-controller` | Unavailable by design and fails clearly: no Kubernetes operator exists before Phase 6 |

After the completion gate, the root Compose services received configurable
local resource ceilings: PostgreSQL defaults to `1.0` CPU/`512m`, Redpanda to
`1.0` CPU/`1g`, and Scheduler to `0.5` CPU/`256m`. These development limits are
documented in `.env.example` and do not define production capacity policy.

## Remaining boundaries and follow-up work

| Severity | Item | Owner phase |
|---|---|---|
| Medium | Phase 3.3 create/get/list API, 3.4 cancellation, and 3.5 retry commands remain absent; their HTTP/authentication and race paths are not claimed by this audit. | Resume Phase 3 |
| Medium | Kubernetes admission and actual node capacity remain authoritative; Scheduler reservations are estimates and the Scheduler creates no Kubernetes resources. | Phase 6 Operator |
| Medium | Request trace persistence is unavailable until the Phase 3 API accepts trace context. Kafka hints propagate existing correlation/trace fields, while polling uses run ID correlation. | Phase 3 API/observability |
| Low | Local PostgreSQL/Redpanda are development-only credentials and topology. Production TLS, ACLs, replicas, autoscaling, alerts, and GitOps manifests remain infrastructure work. | Infrastructure/operations |

## Conclusion

Phase 5 satisfies its Scheduler gate: durable fair claims, recoverable leases,
explained and revalidated quota
admission, validated cluster selection, bounded reservations, atomic state and
events, and an independently runnable bounded process are implemented without
crossing the Phase 6 Kubernetes boundary.
