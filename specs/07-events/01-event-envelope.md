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
