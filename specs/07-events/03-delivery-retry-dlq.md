# Delivery, Retry, and Dead Letters

## Delivery model

At-least-once. Producers write state and outbox intent in one PostgreSQL
transaction. Every consumer stores `(consumer_identity, event_id)` in the same
transaction as its business change and acknowledges the broker only after that
transaction commits.

## Retry classes

- Immediate bounded retry for short network interruptions.
- Delayed retry topics for transient external failures.
- No retry for invalid schema, authorization, policy rejection, or invariant violations.

The adapter owns only bounded immediate transport retries. The outbox owns
durable producer retry. Delayed consumer retries use the `1m`, `5m`, and `30m`
retry topics defined by `07-events/04-event-architecture.md`.

## Dead-letter record

Include original topic/partition/offset/key/headers and envelope, consumer,
error category and code, sanitized message, stack or incident reference,
attempts, first failure, last failure, and operator disposition. Raw secrets,
prompts, source, and authorization data are forbidden.

## Replay

Replay requires authorization and an audit event. The operator chooses a
bounded event set, verifies current aggregate state and schema support, and uses
the original event IDs so idempotent consumers remain safe. An event already in
that consumer's processed-event table remains an acknowledged no-op; successful
markers are not deleted to force a duplicate business effect.

See `07-events/04-event-architecture.md` and ADR-002 for claim leases,
acknowledgement ordering, retry/DLQ topics, replay, retention, and cleanup.

## Phase 4.4 implementation

The PostgreSQL processor exposes the marker transaction explicitly to the
business-effect callback. Broker acknowledgement remains a separate caller
operation after `Process` commits. Two replicas racing on one event rely on the
database primary key; no in-memory lock participates in correctness.

The failure router requires explicit transient/permanent classification when
known. Transient failures publish the unchanged envelope and event ID to the
`1m`, `5m`, then `30m` retry topics before source acknowledgement. Retry
records carry an explicit UTC `retry-not-before` header that retry consumers
must enforce before invoking business logic. Permanent failures and exhausted
transient delivery publish a new
`event-delivery.dead-lettered.v1` envelope. Valid originals are preserved;
malformed bytes are never embedded and are represented by SHA-256 fingerprint,
size, source coordinates, safe failure metadata, and a trusted quarantine
tenant. Publication failure leaves the source unacknowledged.
