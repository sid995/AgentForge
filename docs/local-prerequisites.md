# Local Prerequisites

## Required during Phases 0 through 4.4

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
build metadata, relay, Kafka client, and Redpanda inputs through Phase 4.5.
`.env` is ignored by Git; its included passwords are development-only defaults
and must not be used outside a local machine.

The repository has one root Compose definition. Start the current local
PostgreSQL dependency with `docker compose up --detach postgres`. Start the
local event broker with `docker compose up --detach --wait redpanda`, then run
`make bootstrap-topics`. Stop the local stack with `docker compose down`. Later
phases extend this same
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

## Phase 1 through Phase 4.5 commands

```bash
make help
make check-tools
make format
make verify
make test
make verify-event-contracts
make test-integration
make test-events-integration
make bootstrap-topics
```

`make lint` uses a local `golangci-lint` v2 installation when present. If it is
absent, it runs the pinned `golangci/golangci-lint:v2.9.0` container through the
required Docker daemon. `make test` is an executable Phase 1 quality gate.

`make test-integration` uses the root Compose definition to create an isolated
PostgreSQL 17.5 project on host port `25432`, applies migrations with the local
migration role, runs the tagged database tests with the application and relay
roles, and tears the project down with its test volume. The isolated port and
Compose project avoid changing a developer's normal local stack on port
`15432`. The target supplies deterministic test values rather than relying on
a developer's `.env`. Run `make migrate` against
an already running PostgreSQL instance after setting `AGENTFORGE_DATABASE_URL`
to a migration-role URL.

The Platform API requires `AGENTFORGE_DATABASE_URL` at startup. The bounded
pool controls are `AGENTFORGE_DATABASE_MAX_CONNS` (default `10`),
`AGENTFORGE_DATABASE_MAX_IDLE_CONNS` (default `5`),
`AGENTFORGE_DATABASE_MAX_CONN_LIFETIME` (default `30m`),
`AGENTFORGE_DATABASE_ACQUIRE_TIMEOUT` (default `5s`), and
`AGENTFORGE_DATABASE_CONNECT_TIMEOUT` (default `5s`). Do not use the local
test credentials outside the Compose test environment.

`AGENTFORGE_TEST_RELAY_DATABASE_URL` is integration-test-only and authenticates
the cross-tenant outbox relay role. `POSTGRES_RELAY_PASSWORD` configures that
role when a new local PostgreSQL volume is initialized. Existing volumes do not
rerun initialization scripts; recreate a development volume deliberately if it
predates the relay role and the local data is disposable.

`make test-events-integration` starts only the pinned single-node Redpanda
service in the isolated `agentforge-events-integration` Compose project on host
port `29092`, creates all declared topics, runs the producer/manual-ack consumer
contract tests, and removes the test volume. It fails if Docker, broker health,
topic bootstrap, or Kafka contracts fail. The normal local broker listens on
`REDPANDA_HOST_PORT` (default `19092`). `AGENTFORGE_KAFKA_BROKERS`,
`AGENTFORGE_KAFKA_CLIENT_ID`, `AGENTFORGE_KAFKA_PUBLISH_TIMEOUT`,
`AGENTFORGE_KAFKA_MAX_ATTEMPTS`, and `AGENTFORGE_KAFKA_MAX_MESSAGE_BYTES`
configure clients. Local Redpanda uses plaintext and replication one; it is
only a development/test topology.

`make test-controller` still fails deliberately with a clear message because a
Kubernetes operator has not been introduced.
