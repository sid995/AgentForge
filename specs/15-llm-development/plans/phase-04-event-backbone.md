# Phase 4 Plan: Transactional Outbox and Kafka Backbone

## Status

Phase 4.1 design is committed. Phase 4.2 implements and verifies transactional
AgentRun creation/outbox insertion, competing relay claims, durable retry,
published-only cleanup, and relay shutdown behavior. Canonical paths in this
checkout are `07-events`, `06-data`, `03-domain`, and `15-llm-development`;
they replace older path names in the supplied prompt.

Phase 4.3 now implements the Kafka-compatible producer and manual-ack consumer
base, strict envelope/header/version validation, the first JSON Schema and
fixture, pinned root-Compose Redpanda, topic bootstrap, and a real broker
contract test. Business consumers remain excluded until Phase 4.4.

## Sub-phases

1. Approve the envelope, topics, ownership, delivery, retry/DLQ, retention,
   replay, security, observability, local broker, and contract-test design.
2. Add transactional outbox storage, transaction-aware insertion, competing
   relay claims, durable retry, cleanup, and failure-window tests.
3. Add the Kafka-compatible adapter, schema validation, root-Compose Redpanda,
   topic bootstrap, and producer/consumer contract tests.
4. Add explicit idempotent-consumer transactions, processed-event storage,
   retry classification, DLQ publication, and replica/crash tests.
5. Add compatibility validation and CI, review distributed failure sequences,
   run the Phase 4 gate, and write the completion audit.

Phase 4.2 evidence covers broker absence during the business transaction,
outbox-insert rollback of run/attempt rows, competing claims, relay death after
broker success but before publication marking, stable event IDs on reclaim,
transient retry delay, terminal classification, bounded cleanup, and graceful
publisher close. The broker adapter itself begins in Phase 4.3.

Each validated sub-phase is committed before the next begins. Phase-specific
Compose files are prohibited; Phase 4.3 extends the existing root
`docker-compose.yml` and `.env.example`.

## Dependency boundary

The current repository contains Phase 3.1/3.2 state and persistence but not the
Phase 3.3 create-run API or Phase 3.4/3.5 cancellation/retry commands. Phase 4.2
can make the existing run repository create transaction write
`agent-run.requested.v1`. Cancellation and retry outbox integration cannot be
marked complete until the corresponding command transactions exist. This is an
explicit sequencing risk, not an event-delivery exception.
