# Usage, Audit, and Streaming Services

## Usage service

Consumes model and infrastructure usage events into an append-only ledger. Produces project, tenant, model, and daily aggregates.

## Audit service

Stores immutable security and administrative actions. Audit failures for required actions are fail-closed or durably buffered according to policy.

## Streaming gateway

Provides SSE for run state and selected log notifications. Clients reconnect using a last-event identifier. PostgreSQL remains the durable source; Redis streams or pub/sub provide transient fan-out.
