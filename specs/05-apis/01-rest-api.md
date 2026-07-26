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

Project persistence is implemented in Phase 2, but these tenant-owned HTTP
endpoints remain deferred until Phase 3 introduces an authenticated tenant
resolver. The API must not accept a client-supplied tenant ID or development
header as a substitute.

- `POST /projects`
- `GET /projects/{projectId}`
- `PATCH /projects/{projectId}`
- `GET /projects/{projectId}/runs`

## Runs

Phase 3.3 implements the following authenticated tenant-scoped endpoints. The
executable contract is [`contracts/openapi/platform-api.v1.json`](../../contracts/openapi/platform-api.v1.json).
The temporary development identity accepts only a server-configured opaque
Bearer token and derives the tenant, actor, and role from that configuration;
it never accepts a tenant header or request-body tenant field. It is limited to
`development` and `test` environments and is replaced by OIDC and resource RBAC
in Phase 12.

- `POST /v1/projects/{projectId}/runs` requires `Idempotency-Key`, accepts a
  `promptRef` rather than raw prompt content, creates the run in `QUEUED` with
  its first `PENDING` attempt and requested-event intent atomically, and returns
  `202 Accepted`. Same-tenant key/request replays return the original run;
  differing effective input returns `409 IDEMPOTENCY_KEY_REUSED`.
- `GET /v1/runs/{runId}` returns only a run inside the resolved tenant.
- `GET /v1/projects/{projectId}/runs` returns a tenant/project-scoped page with
  a base64url opaque cursor and `limit` from 1 through 100.

Run responses deliberately exclude the prompt reference, request hash, actor,
and cancellation metadata. The Phase 3.3 request validates `promptRef`,
runtime, CPU/memory request, timeout, and maximum attempts. It does not execute
workloads or accept client status mutations.

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
- A process that is live but cannot reach PostgreSQL returns `503` from
  `/health/ready` with retryable code `DEPENDENCY_UNAVAILABLE`.
