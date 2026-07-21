# Local Prerequisites

## Required during Phase 0 and Phase 1

- Git
- GNU Make or a compatible `make`
- Bash, `awk`, and `find`
- Go 1.26.0 or a compatible patch release
- Docker CLI and a running Docker-compatible daemon for container builds and
  later local dependencies

Run `make check-tools` to verify that the required command-line tools are on
your `PATH`. The check verifies CLI availability; it does not start Docker or
provision cloud resources.

## Required in later phases

Install these only when their governing phase begins:

- `golangci-lint` v2 for Go linting
- ShellCheck for shell-script linting
- kubectl, Helm, and kind for Kubernetes and operator work
- Terraform for cloud infrastructure

The local environment specification adds PostgreSQL, Redis, Redpanda or Kafka,
MinIO, Argo CD, and observability dependencies only in their respective
implementation phases. Do not add them during Phase 0.

## Phase 0 commands

```bash
make help
make check-tools
make format
make verify
```

`make lint` uses a local `golangci-lint` v2 installation when present. If it is
absent, it runs the pinned `golangci/golangci-lint:v2.9.0` container through the
required Docker daemon. `make test` is an executable Phase 1 quality gate.

`make test-integration` and `make test-controller` still fail deliberately with
a clear message because their dependency environments have not been introduced.
