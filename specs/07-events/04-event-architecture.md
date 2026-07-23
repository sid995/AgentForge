# Event Architecture

## Status and implementation boundary

This design governs Phase 4. It uses PostgreSQL as the atomic event-intent
store and Kafka-compatible brokers for at-least-once delivery. Phase 4.1 is
the accepted design; Phase 4.2 implements the transactional outbox, and Phase
4.3 implements the Kafka adapter, executable first-event schema, manual-ack
consumer base, topic bootstrap, and root-Compose Redpanda broker.
Phase 4.4 implements the processed-event transaction, duplicate no-op,
transient retry routing, poison-message DLQ envelope, and sanitized malformed
message quarantine.

Phase 3.1 and 3.2 are present, but the create-run API and cancellation/retry
application commands from Phase 3.3 through 3.5 are not. Phase 4.2 can attach
`agent-run.requested.v1` to the existing transactional run repository create
operation. Cancellation and retry event insertion remain mandatory integration
work once those commands exist; Phase 4 must not claim those command paths are
verified before then.

## Delivery guarantees and failure model

The system provides atomic database state plus event intent, followed by
at-least-once broker publication and at-least-once consumption. It does not
claim exactly-once end-to-end delivery.

1. A business transaction writes aggregate state and an `outbox_events` row.
2. Commit succeeds or fails for both. Broker availability is irrelevant to the
   business commit.
3. A relay durably claims committed outbox rows, publishes them, then marks
   them published in a separate transaction.
4. A crash after broker acknowledgement but before the published update causes
   duplicate publication. Event identity remains unchanged, and consumers must
   deduplicate it.
5. A consumer applies its business change and inserts its processed-event
   marker in one PostgreSQL transaction. It acknowledges the broker offset only
   after that transaction commits.

Kafka ordering is guaranteed only within one topic partition. AgentRun events
use `run_id` as the record key, so events for one run remain ordered while
different runs may interleave. Consumers use `aggregateVersion` to reject stale
events and classify an unexpected version gap as transient until its bounded
retry policy is exhausted. Consumers must not assume ordering across topics,
partitions, aggregates, or replay publications.

## Canonical event envelope

The JSON envelope is UTF-8 and has these fields:

| Field | Required | Contract |
|---|---|---|
| `eventId` | yes | Globally unique UUIDv7 created once with the outbox row and preserved across publication, retry, and replay. A DLQ envelope has its own ID and embeds the original event unchanged. |
| `eventType` | yes | Lowercase dotted domain/action plus major suffix, for example `agent-run.requested.v1`. |
| `schemaVersion` | yes | Positive integer equal to the event type major version. |
| `occurredAt` | yes | UTC RFC 3339 timestamp for the committed domain fact, not relay publication time. |
| `producer` | yes | Stable owning component name such as `platform-api`. |
| `tenantId` | yes | UUIDv7 ownership boundary derived from trusted context. |
| `projectId` | when applicable | UUIDv7 project metadata; never used as the sole authorization input. |
| `runId` | for run events | UUIDv7 run metadata and AgentRun partition key. |
| `aggregateType` | yes | Stable aggregate name such as `AgentRun`. |
| `aggregateId` | yes | Aggregate UUIDv7. |
| `aggregateVersion` | yes | Positive version after the transaction's state change. |
| `requestId` | when command-originated | Valid inbound/generated request ID. |
| `traceparent` | when available | Valid W3C trace context. |
| `tracestate` | when available | Valid W3C trace state after bounded validation. |
| `correlationId` | yes | Workflow correlation ID; defaults to the originating command ID. |
| `causationId` | yes | Direct command/event ID that caused this event. |
| `payload` | yes | JSON object validated by the exact event schema. |

Kafka headers repeat `event-id`, `event-type`, `schema-version`, `tenant-id`,
`correlation-id`, `causation-id`, `request-id`, `traceparent`, and `tracestate`
when present. The body is authoritative, and a mismatch is a permanent invalid
envelope sent to DLQ. Header values are bounded before logging.

Event IDs are not delivery IDs. A retry publication preserves `eventId`; its
delivery attempt and retry timing are transport headers/metadata. A DLQ record
has its own envelope event ID and contains the complete original envelope plus
failure metadata.

## Naming and schema evolution

- Event types use `<domain>.<stable-fact>.v<major>`. Existing hyphenated domains
  and facts such as `agent-run.capacity-wait.v1` remain stable.
- Schemas live in the repository as JSON Schema files under
  `contracts/events/<event-type>.schema.json` once implemented.
- A major version is immutable after publication. Optional fields may be added
  only when old consumers remain valid and the schema permits absence.
- Removing a required field, changing a field type/meaning, narrowing an
  accepted value, or changing partition semantics requires a new major event
  type and a migration window.
- Producers emit one configured version. Consumers explicitly list supported
  versions; unsupported versions are permanent failures and go to DLQ without
  executing business logic.
- Schema files and old supported fixtures remain checked in. CI validates
  examples, producer fixtures, consumer fixtures, and backward compatibility.

## Topics, ownership, and partitioning

Topics use `agentforge.<domain>.<stream>.v<major>` and are environment-scoped
by cluster/account rather than embedding an environment name in application
code.

| Topic | Key | Producers | Consumers | Default retention |
|---|---|---|---|---|
| `agentforge.agent-run.lifecycle.v1` | `runId` | Platform API, Scheduler, Operator, Runner, workflow services for their owned facts | Scheduler, workflow orchestrator, audit/stream projections | 7 days |
| `agentforge.build.lifecycle.v1` | `buildId` | Build service | Workflow orchestrator, audit/stream projections | 7 days |
| `agentforge.deployment.lifecycle.v1` | `projectId:environment` | Deployment service | Workflow orchestrator, audit/stream projections | 14 days |
| `agentforge.governance.events.v1` | owning aggregate ID | Policy, usage, audit, incident owners | Explicit governance projections | 30 days |

Only the owning component may originate a fact. Shared write access to a topic
does not grant ownership of another component's event type. Topic creation,
partition count, replication factor, retention, maximum message bytes, and ACLs
are declarative bootstrap configuration. Partition counts may increase but
must not be decreased in place; consumers never derive business correctness
from a fixed partition number.

Retry topics are `agentforge.<domain>.<stream>.retry.<delay>.v<major>` with the
same record key; the retry segment precedes the single version suffix.
Initial delays are `1m`, `5m`, and `30m`; each retry increments bounded delivery
metadata while preserving the original envelope. Dead letters go to
`agentforge.<domain>.<stream>.dlq.v<major>` keyed by the original partition key. Retry and DLQ
publishing must be acknowledged before the source offset is committed.

## Producer and relay policy

The domain and application layers depend on a small producer port; Kafka client
types remain inside the infrastructure adapter. The adapter uses required
acknowledgements, enables safe idempotent-producer behaviour supported by the
chosen client/broker, preserves per-key ordering, and applies a default
10-second publish timeout configurable only within 1 to 30 seconds. The adapter
may make at most three immediate attempts within that total timeout, using
jittered millisecond backoff only for transient network, leader-election, and
broker-throttling errors. Authorization, message-too-large, invalid topic,
invalid schema, and serialization errors are permanent.

`outbox_events` stores `event_id`, tenant and aggregate metadata, event type and
schema version, physical topic, partition key, complete serialized envelope,
creation time, next-attempt time, publication attempts, claim owner/expiry,
published time, broker partition/offset, last failure category/reason/time, and
a bounded disposition. Event ID is the primary key. The envelope is created and
serialized before the business transaction commits, so serialization failure
rolls back the state change rather than creating an unusable row.

The outbox relay is the durable retry owner:

- Claim a bounded batch ordered by `next_attempt_at, created_at, event_id` using
  `FOR UPDATE SKIP LOCKED`.
- Commit claim owner/expiry before network I/O; never keep a database
  transaction open while publishing.
- A successful publish records `published_at`, broker topic/partition/offset,
  clears the claim, and increments publication attempts.
- A transient failure stores a sanitized reason/category, increments attempts,
  clears the claim, and schedules bounded exponential backoff with jitter.
- Durable producer retry starts at `1s`, then `5s`, `30s`, `2m`, and caps at
  `10m`; it has no finite attempt limit for a valid unpublished event.
- A permanent payload/serialization/configuration failure remains unpublished
  with terminal relay disposition for operator repair; it is never deleted.
- Expired claims are reclaimable after process death. Multiple relay instances
  safely compete, and shutdown stops claims, finishes/cancels bounded in-flight
  publishes, persists results, closes the producer, and exits.

The relay exports dependency-free metric hooks for pending age/count, claims,
publish latency, success, transient/permanent failure, retries, duplicates,
claim expiry, and cleanup. Labels use bounded event type/category values, never
tenant/run/event IDs.

## Consumer transaction and acknowledgement model

Each consumer has a stable identity including its logical name and contract
major version. `processed_events` uses `(consumer_identity, event_id)` as its
primary key and stores tenant, event type/schema version, aggregate metadata,
source topic/partition/offset, processed time, and optional replay metadata.
For every record:

1. Deserialize and validate size, envelope, headers, event type, and schema.
2. Begin the consumer's PostgreSQL business transaction.
3. Insert `(consumer_identity, event_id)` into `processed_events` or detect the
   existing primary key.
4. If duplicate, perform no business effect, commit/rollback cleanly, and
   acknowledge the offset.
5. If new, apply the explicit business operation and commit it with the marker.
6. Acknowledge/commit the broker offset only after the local commit succeeds.

A crash after local commit and before acknowledgement redelivers the record;
the marker converts it into an acknowledged no-op. Two replicas racing on one
event rely on the primary key, not an in-memory mutex. The helper exposes the
transaction and duplicate result but does not hide business transaction
boundaries behind a generic framework.

Transient failures leave the source offset uncommitted until the consumer
publishes to the correct retry topic or retries within its bounded processing
policy. Permanent envelope, schema, authorization, invariant, or payload
failures produce an acknowledged DLQ record. A malformed message that cannot
yield an event ID is fingerprinted for DLQ diagnostics; it never reaches
business logic.

## DLQ, poison messages, and replay

A DLQ envelope contains the original topic, partition, offset, key, headers,
complete original envelope or bounded raw-message reference, consumer identity,
error category/code, sanitized message, stack/incident reference, attempt
count, first/last failure timestamps, and replay disposition. It must not add
raw secrets, prompts, authorization headers, or source content.

Operators quarantine poison messages after the bounded retry schedule. Replay
requires explicit authorization, a ticket/reason, validated current aggregate
state, compatible schema support, and an immutable audit record. Only a bounded
selection is replayed. The original event ID and partition key are preserved.
Events already present in that consumer's `processed_events` remain no-ops;
operators use a compensating command rather than deleting successful markers.
DLQ events that never committed a processed marker may be replayed after the
cause is corrected.

## Retention and cleanup

- Published outbox rows: retain 7 days by default, then delete in bounded
  batches only when `published_at` is non-null and the retention cutoff passed.
- Unpublished, claimed, failed, or terminal-repair outbox rows: never removed by
  automatic cleanup.
- Processed-event markers: retain at least the maximum broker/DLQ replay window,
  default one year. Cleanup is bounded and disabled until the owning consumer's
  replay policy is configured.
- Retry topics: source retention plus the delay and operational buffer, default
  14 days. DLQ topics: 90 days. Machine-readable schemas and compatibility
  fixtures are retained permanently.

## Security and observability

Events may contain identifiers, bounded enums, timestamps, resource metadata,
hashes, and opaque references. They must not contain raw prompts, source files,
secrets, credentials, authorization/cookie headers, unrestricted model output,
or unsanitized failure/stack text. Producers use allowlisted payload structs;
arbitrary maps from client input are prohibited.

Logs include service, event type, consumer/producer, topic, partition, attempt,
request, trace, correlation, and causation metadata where safe. Event ID and
aggregate ID may be structured fields but never metric labels. Traces propagate
valid W3C context in Kafka headers; consumers create linked/child processing
spans according to the tracing implementation phase.

Alerts cover oldest unpublished age, permanent relay failures, retry backlog,
consumer lag, DLQ rate, schema rejection, claim expiry, and cleanup failure.

## Local broker and contract testing

Phase 4.3 extends the existing root `docker-compose.yml` with one pinned
Redpanda broker using the Kafka protocol and health checks. It does not create a
phase-specific Compose file. A reproducible bootstrap command creates topics
with checked-in settings and fails clearly when the broker is unavailable.
All broker environment variables are added to `.env.example`.

Tests use three layers:

- Pure contract tests validate envelope and event fixtures against JSON Schema,
  header/body agreement, version handling, sensitive-field prohibitions, and
  old-version compatibility.
- PostgreSQL integration tests prove atomic outbox writes, competing claims,
  lease recovery, retry scheduling, and cleanup safety.
- Redpanda integration tests prove keyed ordering, duplicate publication,
  producer/consumer shutdown, retry/DLQ routing, duplicate business-effect
  suppression, and crash windows. Tests use deterministic IDs/clocks and do not
  report success when Docker or broker prerequisites are unavailable.

## Phase 4.3 implementation choices

The adapter uses `franz-go` v1.21.5. It keeps Kafka records and error types
inside `internal/adapters/kafka`, uses all in-sync-replica acknowledgements and
the client's default idempotent producer, limits immediate publication to at
most three attempts within the bounded delivery timeout, disables topic auto
creation, and disables consumer auto-commit. Callers explicitly acknowledge a
provider-neutral received record only after their later business transaction.

`jsonschema/v6` v6.0.2 compiles and validates checked-in schemas in contract
tests. Runtime validation remains dependency-light and typed so malformed,
unknown, unsupported-version, topic/key, or header/body mismatches are
permanent records that cannot reach business logic. The dependency adds a test
compilation cost but no broker or schema-registry runtime dependency.

Local development pins Redpanda `v26.1.13` in the existing root Compose file.
It is plaintext, single-node, and replication-one by design and is not a
production security or availability topology. `make test-events-integration`
uses an isolated Compose project and port, bootstraps declared topics, runs the
real producer/consumer contract, and tears down its volume.

## Phase 4.5 compatibility implementation

Every implemented event now has a draft-2020-12 JSON Schema and canonical
example. The published-major baseline records the original required sets and
the JSON signature of every published property. The contract gate rejects a
removed field, any required-set change, or a changed property type, constant,
or reference. Optional additions are allowed only in the live schema; the
baseline remains immutable. Supported v1 consumer fixtures prove old messages
still pass both schema and strict runtime decoding.

Producer contract tests validate the typed `agent-run.requested.v1` and
`event-delivery.dead-lettered.v1` constructors against their schemas. This
found and corrected the DLQ header JSON field casing before publication.
`.github/workflows/repository-checks.yml` runs the compatibility target and the
real Redpanda contract integration independently.

## Phase 6.10 scheduled-intent consumer

The published v1 scheduling fact remains unchanged so older strict consumers
and schemas continue to accept current messages. Its transaction also inserts
a complete immutable `agentrun_handoff_intents` row keyed by event ID. The
handoff checks the durable consumer/event marker first, loads and validates
that exact intent, performs deterministic create-or-compare, then inserts the
marker. A crash after the Kubernetes write but before the marker therefore
repeats only an exact comparison. Transient Kubernetes or database failures
retain the source offset and publish through the bounded retry topics; missing
or invalid intent, tenant/cluster denial, and immutable conflicts are
acknowledged only after DLQ publication.

## Resolved specification contradictions

- The old topic catalog mixed event types with Kafka topics. Event types remain
  the listed facts; this design now defines the separate physical topic names.
- The old replay rule preserved event IDs but did not explain processed-marker
  behaviour. Replays preserve IDs and are intentional no-ops for already
  processed consumer/event pairs; successful markers are never deleted to
  force a second business effect.
- The prior state-machine wording implied Phase 3 already persisted audit and
  event intent. The current checkout implements only Phase 3.1/3.2. Audit rows
  and outbox rows are claimed only when their migrations and transactional
  command integrations exist.
- Immediate producer retry and delayed consumer retry now have separate owners:
  the adapter performs only bounded transport retry, the outbox owns durable
  producer retry, and retry topics own delayed consumer delivery.
