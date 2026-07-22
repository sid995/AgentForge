# ADR-002: Transactional outbox and at-least-once event delivery

- Status: Accepted
- Date: 2026-07-21
- Owners: Platform engineering, Messaging

## Context

AgentForge changes authoritative workflow state in PostgreSQL and must publish
integration events without a database/broker dual-write gap. Kafka-compatible
delivery can duplicate records during producer and consumer crash windows, and
services must remain available when the broker is temporarily unavailable.

## Decision

Write each event intent to PostgreSQL in the same transaction as its aggregate
change. A separate relay claims committed rows in bounded batches, performs no
network I/O inside a database transaction, and publishes to Kafka-compatible
topics at least once. Broker publication success and the outbox published mark
are separate operations, so all consumers must be idempotent.

Consumers insert a stable `(consumer_identity, event_id)` marker in the same
local transaction as their business effect and acknowledge offsets only after
commit. Permanent failures are published to a DLQ before the source offset is
acknowledged. Local development uses a pinned single-node Redpanda service in
the repository's root Compose file; production may use any approved
Kafka-compatible service satisfying the contract.

Event types and JSON schemas are versioned independently from physical topics.
Provider-specific Kafka client types stay inside adapters. PostgreSQL remains
authoritative; Kafka, retries, and projections never become the workflow source
of truth.

## Alternatives considered

- Synchronous database then broker writes: rejected because either ordering
  loses events or publishes events for rolled-back state.
- Distributed transactions/Kafka exactly-once claims: rejected because they do
  not atomically include the PostgreSQL business transaction and would obscure
  required duplicate handling.
- Database polling as the permanent integration API: rejected because it
  couples consumers to private schemas and cannot provide governed event
  contracts.
- Deleting processed markers for replay: rejected because it can repeat
  successful business effects; compensating commands are explicit and safer.

## Consequences

Publication is eventually consistent and duplicates are normal. Outbox rows,
claims, processed markers, retry/DLQ records, compatibility schemas, lag
metrics, and operator replay procedures are required. Broker downtime increases
backlog but does not fail a committed API command. Unpublished rows are never
automatically cleaned up.

## Validation

Required tests cover broker outage after database commit, publish/mark crash,
competing relays, relay restart, duplicate publication, malformed payloads,
cleanup safety, consumer commit/ack crash, two consumer replicas, transient
retry, permanent DLQ, and schema compatibility.

## Revisit conditions

Revisit if the authoritative store changes, broker semantics cannot meet the
documented acknowledgement/durability contract, or a proven end-to-end atomic
protocol can replace the outbox without weakening failure recovery.
