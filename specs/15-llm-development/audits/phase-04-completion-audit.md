# Phase 4 Completion Audit: Transactional Outbox and Kafka Backbone

**Audited:** 2026-07-21

## Scope reviewed

- ADR-002 and the Phase 4 event architecture, envelope, topic, retry/DLQ, data,
  security, testing, and CI specifications
- AgentRun/outbox transaction, relay claims and lifecycle, Kafka producer and
  manual-ack consumer, processed-event transaction, retry/DLQ router, schemas,
  compatibility baselines, fixtures, root Compose, and topic bootstrap
- PostgreSQL and Redpanda failure-window tests, race/static checks, repository
  controls, documentation, status, and traceability

The supplied audit path used the obsolete `12-llm-development` prefix. The
canonical manifest and execution playbook require this file under
`15-llm-development/audits/`.

## Phase gate evidence

| Requirement | Evidence | Result |
|---|---|---|
| Run creation writes event intent atomically | `AgentRunRepository.Create` inserts run, first attempt, and `agent-run.requested.v1` outbox row in one tenant transaction; revoking outbox insert proves every business row rolls back | Passed |
| Broker downtime does not lose committed events | The business transaction has no broker call; an unpublished row remains claimable after a simulated broker failure and is excluded from published-only cleanup | Passed |
| Duplicate delivery does not duplicate business effects | `(consumer_identity,event_id)` is the primary key; two concurrent replicas and a post-commit/pre-ack redelivery produce one marker and one effect | Passed |
| Event schemas are versioned and compatible | Both implemented event types have draft-2020-12 schemas, canonical examples, immutable-major baselines, producer tests, and supported v1 consumer fixtures | Passed |
| Unsupported schemas are deliberate failures | Strict decode rejects mismatched or unsupported major/schema versions before business logic; invalid bytes use fingerprint-only quarantine | Passed |
| Retry and DLQ behavior is tested | Router tests cover transient, permanent, exhausted, malformed, and publish-failure paths; Redpanda proves retry and DLQ records are publishable and consumable | Passed |
| Replay is safe and documented | Replay is authorized, bounded, schema/state checked, preserves event ID/key, and never deletes a successful processed marker to force a second effect | Passed |
| Status and traceability are current | `PROJECT-STATUS.md`, `TRACEABILITY.md`, event/data/security/testing/CI specs, and the Phase 4 plan point to concrete code and test evidence | Passed |

## Distributed-systems failure review

### Dual-write and broker outage

Failure sequence: PostgreSQL commits a run, then the broker is unavailable.
There is no dual write: the same database transaction already committed the
serialized outbox intent. The relay records a sanitized transient failure and
schedules the same event ID for durable retry. Cleanup selects only published
rows, so outage duration does not silently discard the event.

Failure sequence: outbox insertion fails after run/attempt statements. The
transaction returns an error and rolls back all three records. A run without its
event intent cannot become visible.

### Duplicate publication and relay concurrency

Failure sequence: Kafka acknowledges publication, then the relay dies before
`published_at` commits. The lease expires and another relay republishes the same
serialized envelope and event ID. This is an expected at-least-once duplicate,
not a lost event. Consumers use the event ID marker, not offsets or in-memory
locks, for correctness.

Failure sequence: two relays poll the same due rows. `FOR UPDATE SKIP LOCKED`
and committed claim owner/expiry metadata assign a row to one live lease. The
loser cannot mark the row because update predicates include its claim owner.

### Consumer transaction and acknowledgement ordering

Failure sequence: a consumer commits its marker and business effect, then dies
before committing the Kafka offset. Redelivery encounters the primary-key
marker, commits a duplicate no-op, and can then acknowledge. If the business
effect fails, marker and effect roll back together, so a retry can execute once.

The Kafka adapter disables auto-commit and exposes acknowledgement separately.
Retry or DLQ publication must return successfully before the caller invokes
acknowledgement; publication failure leaves the source offset uncommitted.

### Ordering, retries, and poison messages

AgentRun records use `runId` as their Kafka key. Ordering claims are limited to
one key within one partition; consumers still validate aggregate versions and
must not infer global ordering. Producer retries are bounded by three attempts
and a ten-second default deadline. Durable outbox retry is independently
scheduled, preventing an in-memory retry loop.

Transient consumer failures progress once through `1m`, `5m`, and `30m`
topics. The router validates source topic plus prior `delivery-attempt`, and
retry consumers must enforce `retry-not-before`; attempt four dead-letters
instead of cycling. Permanent validation/invariant failures skip retry.
Malformed bytes are not copied into the DLQ: only size, source coordinates, and
a SHA-256 fingerprint are retained under a trusted quarantine tenant.

### Schema evolution and sensitive data

Compatibility validation rejects removal of published fields, required-set
changes, and incompatible JSON type, constant, or reference changes within v1.
Only optional live-schema additions are allowed without a new major. Producer
and supported-old-consumer fixtures are both validated. Runtime decoding still
rejects unknown fields and unsupported versions before business code.

Payloads use allowlisted execution metadata and prompt references. Failure
messages are explicitly safe and bounded. Original DLQ headers are allowlisted;
authorization and arbitrary malformed bytes are excluded. The compatibility
producer test exposed uppercase `Key`/`Value` JSON names in original DLQ header
metadata; Phase 4.5 corrected them to the schema-defined `key`/`value` form.

### Cleanup and replay

Outbox cleanup is bounded and deletes only rows with a published disposition
older than the retention boundary. Terminal/unpublished events survive for
operator repair. Replay requires authorization, current-state/schema checks,
an immutable audit record in the later governance implementation, and a bounded
selection. Existing processed markers intentionally make already-successful
replays no-ops.

## Validation record

| Command | Result |
|---|---|
| `make check-tools` | Passed |
| `make format` | Passed |
| `make lint` | Passed |
| `make test` | Passed |
| `make verify-event-contracts` | Passed for schemas, examples, compatibility, producers, and consumer fixtures |
| `go test -race ./services/platform-api/...` | Passed |
| `go vet ./services/platform-api/...` | Passed |
| `make verify` | Passed |
| `docker compose config -q` | Passed for the single root Compose file |
| `make test-integration` | Passed against fresh PostgreSQL with migrations 1 through 4 |
| `make test-events-integration` | Passed against isolated pinned Redpanda and bootstrapped topics |
| `make build-platform-api` | Passed |

## Remaining boundaries and follow-up work

| Severity | Item | Owner phase |
|---|---|---|
| Medium | The Phase 3.3 authenticated create/get/list API is not implemented. Atomic create/outbox evidence is at the repository transaction boundary, not an HTTP path. | Resume Phase 3.3 |
| Medium | Cancellation and retry command transactions do not exist, so their specified outbox events cannot yet be wired. | Phase 3.4 and 3.5 |
| Medium | Relay and consumer foundations are executable code paths but are not yet wired into deployed production workloads; business consumers remain intentionally absent. | Scheduler/workflow and deployment phases |
| Medium | Authorized replay requires the later identity, governance audit, and operator tooling; Phase 4 defines and tests the idempotency semantics only. | Identity/governance/operations phases |
| Low | Local Redpanda is plaintext, single-node, and replication-one. Production ACL, TLS, replication, capacity, and alerting remain infrastructure work. | Infrastructure and operations phases |

## Conclusion

The Phase 4 foundation satisfies its completion gate without claiming the
deferred Phase 3 APIs, business consumers, replay tooling, or production broker
topology. Transactional intent, at-least-once publication semantics,
idempotent consumption, bounded retry/DLQ handling, and contract compatibility
are implemented and verified. The next safe product step is to resume Phase
3.3 at the authenticated API boundary.
