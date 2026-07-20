# API Versioning and Idempotency

## Versioning

- Major API version in URL.
- Additive fields are backward-compatible.
- Removing or changing semantics requires a new major version or migration period.
- Clients must ignore unknown response fields.

## Idempotency

For selected mutating requests, store tenant, key, request hash, status, and serialized response.

Rules:

- Same key and same request returns the prior response.
- Same key and different request returns 409.
- Keys expire only after the maximum safe replay window.
- Downstream commands include stable command IDs derived from the original operation.
