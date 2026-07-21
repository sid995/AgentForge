# Local Prerequisites

## Required during Phases 0 through 3

- Git
- GNU Make or a compatible `make`
- Bash, `awk`, and `find`
- Go 1.26.0 or a compatible patch release
- Docker CLI and a running Docker-compatible daemon for container builds and
  local PostgreSQL integration tests

Run `make check-tools` to verify that the required command-line tools are on
your `PATH`. The check verifies CLI availability; it does not start Docker or
provision cloud resources.

## Local environment configuration

Copy [`.env.example`](../.env.example) to `.env` before running the local
PostgreSQL dependency. The example documents every environment variable used
by the Platform API, migration command, Compose configuration, and supported
build metadata inputs through Phase 3.2. `.env` is ignored by Git; its included
passwords are development-only defaults and must not be used outside a local
machine.

The repository has one root Compose definition. Start the current local
PostgreSQL dependency with `docker compose up --detach postgres` and stop it
with `docker compose down`. Later phases extend this same
[`docker-compose.yml`](../docker-compose.yml); they must not add phase-specific
Compose files.

## Required in later phases

Install these only when their governing phase begins:

- `golangci-lint` v2 for Go linting
- ShellCheck for shell-script linting
- kubectl, Helm, and kind for Kubernetes and operator work
- Terraform for cloud infrastructure

The local environment specification adds PostgreSQL, Redis, Redpanda or Kafka,
MinIO, Argo CD, and observability dependencies only in their respective
implementation phases. Do not add them during Phase 0.

## Phase 1 through Phase 3.2 commands

```bash
make help
make check-tools
make format
make verify
make test
make test-integration
```

`make lint` uses a local `golangci-lint` v2 installation when present. If it is
absent, it runs the pinned `golangci/golangci-lint:v2.9.0` container through the
required Docker daemon. `make test` is an executable Phase 1 quality gate.

`make test-integration` uses the root Compose definition to create an isolated
PostgreSQL 17.5 project on host port `15432`, applies migrations with the local
migration role, runs the tagged database tests with the application role, and
tears the project down with its test volume. It supplies deterministic test
values rather than relying on a developer's `.env`. Run `make migrate` against
an already running PostgreSQL instance after setting `AGENTFORGE_DATABASE_URL`
to a migration-role URL.

The Platform API requires `AGENTFORGE_DATABASE_URL` at startup. The bounded
pool controls are `AGENTFORGE_DATABASE_MAX_CONNS` (default `10`),
`AGENTFORGE_DATABASE_MAX_IDLE_CONNS` (default `5`),
`AGENTFORGE_DATABASE_MAX_CONN_LIFETIME` (default `30m`),
`AGENTFORGE_DATABASE_ACQUIRE_TIMEOUT` (default `5s`), and
`AGENTFORGE_DATABASE_CONNECT_TIMEOUT` (default `5s`). Do not use the local
test credentials outside the Compose test environment.

`make test-controller` still fails deliberately with a clear message because a
Kubernetes operator has not been introduced.
