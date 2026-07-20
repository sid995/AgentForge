# Model Gateway

## Responsibilities

- Authenticate workload identity.
- Verify tenant, run, model, and budget authorization.
- Normalize provider APIs through adapters.
- Count tokens and calculate cost.
- Apply rate limits, retries, circuit breakers, and fallbacks.
- Redact sensitive telemetry.

## Routing policies

- Fixed model.
- Cheapest model satisfying capability.
- Latency-optimized model.
- Primary with fallback.
- Budget-aware downgrade.

## Failure rules

- Respect provider retry hints.
- Retry only before a response is considered committed.
- Record provider request IDs.
- Open a per-provider circuit after configured failure thresholds.
- Isolate provider concurrency with separate bulkheads.
