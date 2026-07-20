# Event Envelope

```json
{
  "eventId": "0191...",
  "eventType": "agent-run.started.v1",
  "occurredAt": "2026-07-20T16:01:00Z",
  "producer": "agent-operator",
  "tenantId": "0190...",
  "aggregateType": "AgentRun",
  "aggregateId": "0191...",
  "aggregateVersion": 4,
  "traceId": "...",
  "causationId": "...",
  "correlationId": "...",
  "payload": {}
}
```

## Rules

- Event ID is globally unique.
- Event type contains explicit version.
- Payload schemas are immutable after publication.
- Sensitive prompts, source, and secrets are referenced, not embedded.
- Correlation follows the full workflow; causation identifies the direct triggering command or event.
