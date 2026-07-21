# Event Envelope

```json
{
  "eventId": "0191...",
  "eventType": "agent-run.started.v1",
  "schemaVersion": 1,
  "occurredAt": "2026-07-20T16:01:00Z",
  "producer": "agent-operator",
  "tenantId": "0190...",
  "projectId": "0190...",
  "runId": "0191...",
  "aggregateType": "AgentRun",
  "aggregateId": "0191...",
  "aggregateVersion": 4,
  "requestId": "req_...",
  "traceparent": "00-...-...-01",
  "causationId": "...",
  "correlationId": "...",
  "payload": {}
}
```

## Rules

- Event ID is globally unique.
- Event type contains an explicit major version and `schemaVersion` matches it.
- Payload schemas are immutable after publication.
- Sensitive prompts, source, and secrets are referenced, not embedded.
- Correlation follows the full workflow; causation identifies the direct triggering command or event.
- Kafka headers repeat bounded routing and correlation metadata. The body is
  authoritative, and mismatches are permanent envelope failures.

The complete field, header, validation, and evolution contract is defined in
`07-events/04-event-architecture.md`.

The implemented `agent-run.requested.v1` and
`event-delivery.dead-lettered.v1` contracts are checked in under
`contracts/events/` with canonical examples. Runtime decoding rejects unknown
envelope and payload fields, mismatched major/schema versions, unsupported event
types, invalid UUIDv7 identities, invalid trace context, and header/body
mismatches.

## Compatibility controls

Published-major baselines live in `contracts/events/compatibility/`; supported
old consumer inputs live in `contracts/events/fixtures/consumer/<major>/`.
`make verify-event-contracts` compiles every implemented schema, validates its
canonical example and producer output, passes supported fixtures through both
the schema and runtime decoder, and compares the live schema with its baseline.

Within one major, existing properties and required sets are immutable. An
optional property may be added when its absence remains valid and consumers
ignore unknown optional data at their integration boundary. Removing a field,
changing its JSON type, constant, or reference, or changing requiredness needs
a new event-type major and a parallel migration window. Baselines and supported
fixtures are never rewritten merely to make an incompatible change pass.
