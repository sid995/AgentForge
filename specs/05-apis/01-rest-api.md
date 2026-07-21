# REST API Contract

Base path: `/v1`

## Service health

These endpoints are intentionally outside `/v1` so load balancers and
orchestrators can probe the process before API authentication is introduced.

- `GET /health/live` returns `200` while the process can serve HTTP. It never
  checks external dependencies.
- `GET /health/ready` returns `200` only after the listener is bound and `503`
  during startup or graceful shutdown. It is process-only in Phase 1; each
  mandatory dependency will add its owned readiness check when introduced.

Both endpoints return `application/json; charset=utf-8` with:

```json
{
  "status": "ok",
  "service": "platform-api",
  "version": "development",
  "commit": "unknown",
  "buildTime": "unknown"
}
```

## Projects

- `POST /projects`
- `GET /projects/{projectId}`
- `PATCH /projects/{projectId}`
- `GET /projects/{projectId}/runs`

## Runs

- `POST /projects/{projectId}/runs`
- `GET /runs/{runId}`
- `POST /runs/{runId}/cancel`
- `POST /runs/{runId}/retry`
- `GET /runs/{runId}/attempts`
- `GET /runs/{runId}/trajectory`
- `GET /runs/{runId}/artifacts`
- `GET /runs/{runId}/events`

## Builds and deployments

- `GET /builds/{buildId}`
- `POST /builds/{buildId}/deployments`
- `GET /deployments/{deploymentId}`
- `POST /deployments/{deploymentId}/approve`
- `POST /deployments/{deploymentId}/rollback`

## Operations

- `GET /operations/clusters`
- `GET /operations/queues`
- `POST /operations/runs/{runId}/terminate`

## Common behavior

- Mutating commands accept `Idempotency-Key`.
- Responses include `X-Request-ID`.
- Pagination uses opaque cursor tokens.
- Errors use a stable structured envelope.
- Requests receive an `X-Request-ID`; a valid supplied value is propagated.
- A valid W3C `traceparent` header is passed through for later OpenTelemetry
  integration.
- Request bodies are limited to 1 MiB by default and requests time out after
  30 seconds by default. Limits are configured only through bounded service
  environment variables.
- Malformed routes, oversized requests, timeouts, and unexpected failures use
  the structured error envelope. Their stable codes are `NOT_FOUND`,
  `REQUEST_TOO_LARGE`, `REQUEST_TIMEOUT`, and `INTERNAL`.
