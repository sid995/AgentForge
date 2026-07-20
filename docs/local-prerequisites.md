# Local Prerequisites

## Required during Phase 0 and Phase 1

- Git
- GNU Make or a compatible `make`
- Bash, `awk`, and `find`
- Go, using the version selected when the Phase 1 workspace is initialized
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

`make lint`, `make test`, `make test-integration`, and `make test-controller`
currently fail deliberately with a clear message because no product source or
test environment exists. They become executable quality gates when the
corresponding implementation is introduced.
