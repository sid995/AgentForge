# REST API Contract

Base path: `/v1`

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
