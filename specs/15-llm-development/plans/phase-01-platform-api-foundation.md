# Phase 1 Plan: Platform API Foundation

## Status

Implemented and validated on 2026-07-21. This plan implements the first
deployable Platform API only; it does not introduce persistence, broker
clients, authentication, or tenant-owned business resources.

## Governing inputs

- `AGENTS.md`
- `specs/02-architecture/01-system-context.md`
- `specs/02-architecture/02-container-architecture.md`
- `specs/02-architecture/04-design-patterns.md`
- `specs/04-services/01-platform-api.md`
- `specs/05-apis/01-rest-api.md`
- `specs/05-apis/02-api-examples.md`
- `specs/05-apis/03-versioning-and-idempotency.md`
- `specs/09-security/01-threat-model.md`
- `specs/10-observability/01-telemetry-standards.md`
- `specs/12-testing/01-test-strategy.md`
- `specs/15-llm-development/05-codex-execution-playbook.md`

## Scope and boundaries

This phase delivers a Go workspace and one `platform-api` process with:

- deterministic configuration from environment variables;
- JSON structured logs without request bodies, authorization headers, prompts,
  or secret values;
- liveness and readiness endpoints;
- build and version information;
- request correlation, trace-context passthrough, timeout, panic recovery, and
  bounded request-body middleware;
- a stable JSON error envelope; and
- graceful process shutdown.

It deliberately excludes PostgreSQL, Redis, Kafka, OpenTelemetry SDK export,
authentication, authorization, tenant resolution, projects, runs, and all
other business endpoints. The health endpoints do not assert availability of
dependencies that this phase does not create.

## Package structure

```text
go.work
services/platform-api/
  go.mod
  Dockerfile
  cmd/platform-api/main.go
  internal/config/config.go
  internal/httpapi/server.go
  internal/httpapi/health.go
  internal/httpapi/middleware.go
  internal/httpapi/errors.go
  internal/buildinfo/buildinfo.go
  internal/logging/logger.go
```

- `cmd/platform-api` composes dependencies and owns process lifetime.
- `internal/config` parses and validates bounded environment configuration.
- `internal/httpapi` exposes transport-only handlers and standard-library
  middleware.
- `internal/buildinfo` supplies compile-time build metadata with safe local
  defaults.
- `internal/logging` constructs the shared `log/slog` JSON logger.

The phase uses Go's standard library only. `net/http`, `log/slog`, and
`signal.NotifyContext` are sufficient for the required behavior, avoid an
unjustified framework dependency, and retain a small surface for later ports
and adapters.

## Configuration model

| Variable | Default | Validation | Purpose |
|---|---|---|---|
| `AGENTFORGE_HTTP_ADDR` | `:8080` | valid host:port | listener address |
| `AGENTFORGE_ENVIRONMENT` | `development` | non-empty, bounded string | log and service context |
| `AGENTFORGE_SHUTDOWN_TIMEOUT` | `10s` | positive duration | maximum graceful-shutdown wait |
| `AGENTFORGE_REQUEST_TIMEOUT` | `30s` | positive duration | request deadline |
| `AGENTFORGE_MAX_REQUEST_BODY_BYTES` | `1048576` | positive integer | request-body limit |

Build metadata is injected at build time using linker flags, with explicit
`development` and `unknown` fallbacks. Invalid configuration prevents startup
and logs only the variable name and validation error, never its raw value.

## HTTP behavior

All responses include `X-Request-ID`; all JSON responses include
`Content-Type: application/json; charset=utf-8`.

| Endpoint | Success | Failure behavior |
|---|---|---|
| `GET /health/live` | `200` with service and build metadata | Never probes dependencies; failure means this process cannot serve HTTP. |
| `GET /health/ready` | `200` with service and build metadata once startup completes | `503` during startup or shutdown; dependency checks are added with their owning dependency phase. |

Middleware order will establish/generate request ID, pass through a valid W3C
`traceparent` header without exporting spans, enforce body and request-time
bounds, recover panics, write errors, and log final status, duration, request
ID, trace ID, method, and route. Request bodies and authorization headers are
never logged. Oversized bodies return `413`; deadline expiry returns `504`;
unexpected panics return a non-sensitive `500` error.

The stable error form remains compatible with `05-apis/02-api-examples.md`:

```json
{
  "error": {
    "code": "INTERNAL",
    "message": "An internal error occurred.",
    "requestId": "req_...",
    "retryable": false
  }
}
```

## Development identity boundary

Phase 1 provides no identity mechanism and no tenant-owned command or query.
It must reject no authorization based on client-supplied tenant information
because it accepts no such information. Authentication, tenant resolution,
authorization, and tenant-scoped persistence are deferred to their governing
phases; Phase 1 must not add a development header or environment shortcut that
could later become an identity bypass.

## Graceful shutdown

The entrypoint listens only after configuration and dependencies are built. On
`SIGINT` or `SIGTERM`, it marks readiness unavailable, stops accepting new
connections with `http.Server.Shutdown`, and waits up to
`AGENTFORGE_SHUTDOWN_TIMEOUT` for in-flight handlers. A timeout or listener
failure is logged with structured context and produces a non-zero exit code.

## Test plan

- Configuration defaults, overrides, and invalid values.
- Liveness and readiness contracts, including readiness during shutdown.
- Build metadata response fields.
- Generated and propagated request IDs.
- Trace-context passthrough without trace export.
- JSON content type and error-envelope shape.
- Request-timeout cancellation and `504` mapping.
- Panic recovery without leaked panic details.
- Request-body limit and `413` mapping.
- Request logs contain correlation and service fields but exclude sensitive
  headers and body content.
- Start/stop lifecycle tests using an ephemeral listener.

`make test` runs Go unit tests. `make lint` runs `golangci-lint` when
available. `make test-integration` and `make test-controller` remain explicit
failures because this phase has neither external dependencies nor a
controller. The Dockerfile is validated by a local container build when Docker
is available.

## Planned files and updates

- Create `go.work` and `services/platform-api/go.mod`.
- Create the Platform API source, tests, and `Dockerfile` listed above.
- Update `Makefile` for workspace-aware formatting, linting, and unit tests.
- Update `specs/05-apis/01-rest-api.md` and
  `specs/05-apis/02-api-examples.md` with health and error contracts.
- Update `specs/PROJECT-STATUS.md` and `specs/TRACEABILITY.md` only after
  implementation and validation evidence exists.
- Add a Phase 1 completion audit only after all acceptance checks are proven.

## Acceptance criteria

- The Platform API starts and terminates cleanly.
- `/health/live` and `/health/ready` behave as documented.
- Structured logs include service and request-correlation context and exclude
  sensitive payloads.
- Unit tests and static analysis pass.
- The service container image builds successfully.

## Risks and deferred work

- Readiness is process-only until PostgreSQL is introduced in Phase 2; it must
  be extended to check mandatory dependencies then.
- Trace context is propagated but not exported until the observability phase.
- No endpoint can claim durable command acceptance until PostgreSQL, migrations,
  and transactional outbox support exist.
- OIDC, RBAC, tenant resolution, and repository/RLS enforcement remain future
  work and must precede any tenant-owned API.
