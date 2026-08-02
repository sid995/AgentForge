SHELL := /bin/bash

.DEFAULT_GOAL := help

GOLANGCI_LINT_IMAGE := golangci/golangci-lint:v2.9.0
BUILD_VERSION ?= development
BUILD_COMMIT ?= unknown
BUILD_TIME ?= unknown

.PHONY: help check-tools format lint lint-controller test verify-event-contracts test-integration test-events-integration test-runner-integration test-artifact-integration test-controller test-controller-kind operator-manifests operator-generate migrate bootstrap-topics build-platform-api build-scheduler build-handoff build-operator build-runner check-all check-all-kind verify

help:
	@printf '%s\n' 'AgentForge development targets:'
	@printf '%s\n' '  check-tools       Check Phase 0 through Phase 6 prerequisites'
	@printf '%s\n' '  format            Format tracked Go source'
	@printf '%s\n' '  lint              Run Platform API, Agent Runner, and Operator static analysis'
	@printf '%s\n' '  lint-controller   Run Operator static analysis'
	@printf '%s\n' '  test              Run Platform API and Agent Runner unit tests'
	@printf '%s\n' '  verify-event-contracts Validate event schemas, compatibility, and fixtures'
	@printf '%s\n' '  test-integration  Run PostgreSQL integration tests in Docker Compose'
	@printf '%s\n' '  test-events-integration Run Kafka contract tests against isolated Redpanda'
	@printf '%s\n' '  test-runner-integration Run Agent Runner artifact tests against isolated MinIO'
	@printf '%s\n' '  test-artifact-integration Run Platform API artifact-store tests against isolated MinIO'
	@printf '%s\n' '  test-controller   Generate manifests and run Operator envtest coverage'
	@printf '%s\n' '  test-controller-kind Run the pinned Operator kind lifecycle gate'
	@printf '%s\n' '  operator-manifests Regenerate Operator CRD and RBAC manifests'
	@printf '%s\n' '  operator-generate Regenerate Operator Go code'
	@printf '%s\n' '  migrate           Apply checked-in PostgreSQL migrations'
	@printf '%s\n' '  bootstrap-topics  Create/update local Redpanda topics'
	@printf '%s\n' '  build-platform-api Build the Platform API container image (BUILD_VERSION, BUILD_COMMIT, BUILD_TIME are supported)'
	@printf '%s\n' '  build-scheduler    Build the Scheduler container image (BUILD_VERSION, BUILD_COMMIT, BUILD_TIME are supported)'
	@printf '%s\n' '  build-handoff      Build the AgentRun handoff container image (BUILD_VERSION, BUILD_COMMIT, BUILD_TIME are supported)'
	@printf '%s\n' '  build-operator     Build the Agent Operator container image'
	@printf '%s\n' '  build-runner       Build the deterministic Agent Runner container image'
	@printf '%s\n' '  check-all          Run all local checks, integrations, and image builds except the kind gate'
	@printf '%s\n' '  check-all-kind     Run check-all plus the pinned Kubernetes kind lifecycle gate'
	@printf '%s\n' '  verify            Validate repository controls and documentation inventory'

check-tools:
	@scripts/check-tools.sh

format:
	@if ! find . -type f -name '*.go' -not -path './.git/*' -print -quit | grep -q .; then \
		printf '%s\n' 'No Go source files exist yet; nothing to format.'; \
	else \
		find . -type f -name '*.go' -not -path './.git/*' -exec gofmt -w {} +; \
	fi

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd services/platform-api && golangci-lint run ./...; \
	else \
		docker run --rm --volume "$(CURDIR):/workspace" --workdir /workspace/services/platform-api $(GOLANGCI_LINT_IMAGE) golangci-lint run ./...; \
	fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd agent-runner && golangci-lint run ./...; \
	else \
		docker run --rm --volume "$(CURDIR):/workspace" --workdir /workspace/agent-runner $(GOLANGCI_LINT_IMAGE) golangci-lint run ./...; \
	fi
	@$(MAKE) -C operator lint

lint-controller:
	@$(MAKE) -C operator lint

test:
	@go test ./services/platform-api/...
	@go test ./agent-runner/...

verify-event-contracts:
	@go test -count=1 ./services/platform-api/internal/events -run '^TestEventContract'

test-integration:
	@set -euo pipefail; \
		cleanup() { docker compose -f docker-compose.yml -p agentforge-integration-test down --volumes --remove-orphans; }; \
		trap cleanup EXIT; \
		POSTGRES_DB=agentforge POSTGRES_USER=agentforge_migrator POSTGRES_PASSWORD=agentforge-migrator POSTGRES_APP_PASSWORD=agentforge-app POSTGRES_RELAY_PASSWORD=agentforge-relay POSTGRES_SCHEDULER_PASSWORD=agentforge-scheduler POSTGRES_HANDOFF_PASSWORD=agentforge-handoff POSTGRES_HOST_PORT=25432 docker compose -f docker-compose.yml -p agentforge-integration-test up --detach --wait postgres; \
		AGENTFORGE_DATABASE_URL='postgres://agentforge_migrator:agentforge-migrator@127.0.0.1:25432/agentforge?sslmode=disable' go run ./services/platform-api/cmd/migrate; \
		AGENTFORGE_TEST_DATABASE_URL='postgres://agentforge_migrator:agentforge-migrator@127.0.0.1:25432/agentforge?sslmode=disable' \
		AGENTFORGE_TEST_APP_DATABASE_URL='postgres://agentforge_app:agentforge-app@127.0.0.1:25432/agentforge?sslmode=disable' \
		AGENTFORGE_TEST_RELAY_DATABASE_URL='postgres://agentforge_relay:agentforge-relay@127.0.0.1:25432/agentforge?sslmode=disable' \
		AGENTFORGE_TEST_SCHEDULER_DATABASE_URL='postgres://agentforge_scheduler:agentforge-scheduler@127.0.0.1:25432/agentforge?sslmode=disable' \
		AGENTFORGE_TEST_HANDOFF_DATABASE_URL='postgres://agentforge_handoff:agentforge-handoff@127.0.0.1:25432/agentforge?sslmode=disable' \
		go test -count=1 -tags=integration ./services/platform-api/...

test-events-integration:
	@set -euo pipefail; \
		cleanup() { docker compose -f docker-compose.yml -p agentforge-events-integration down --volumes --remove-orphans; }; \
		trap cleanup EXIT; \
		REDPANDA_HOST_PORT=29092 docker compose -f docker-compose.yml -p agentforge-events-integration up --detach --wait redpanda; \
		COMPOSE_PROJECT_NAME=agentforge-events-integration scripts/bootstrap-topics.sh; \
		AGENTFORGE_TEST_KAFKA_BROKERS='127.0.0.1:29092' go test -count=1 -tags=brokerintegration ./services/platform-api/internal/adapters/kafka

test-runner-integration:
	@set -euo pipefail; \
		cleanup() { docker compose -f docker-compose.yml -p agentforge-runner-integration --profile runner down --volumes --remove-orphans; }; \
		trap cleanup EXIT; \
		runner_access_key="$${RUNNER_MINIO_ROOT_USER:-agentforge-runner}"; runner_secret_key="$${RUNNER_MINIO_ROOT_PASSWORD:-agentforge-runner-secret}"; \
		RUNNER_MINIO_HOST_PORT=29000 RUNNER_MINIO_ROOT_USER="$$runner_access_key" RUNNER_MINIO_ROOT_PASSWORD="$$runner_secret_key" docker compose -f docker-compose.yml -p agentforge-runner-integration --profile runner up --detach --wait minio; \
		AGENTFORGE_TEST_MINIO_ENDPOINT='127.0.0.1:29000' AGENTFORGE_TEST_MINIO_ACCESS_KEY="$$runner_access_key" AGENTFORGE_TEST_MINIO_SECRET_KEY="$$runner_secret_key" go test -count=1 -tags=runnerintegration ./agent-runner/internal/runner

test-artifact-integration:
	@set -euo pipefail; \
		cleanup() { docker compose -f docker-compose.yml -p agentforge-artifact-integration --profile runner down --volumes --remove-orphans; }; \
		trap cleanup EXIT; \
		runner_access_key="$${RUNNER_MINIO_ROOT_USER:-agentforge-runner}"; runner_secret_key="$${RUNNER_MINIO_ROOT_PASSWORD:-agentforge-runner-secret}"; \
		RUNNER_MINIO_HOST_PORT=29000 RUNNER_MINIO_ROOT_USER="$$runner_access_key" RUNNER_MINIO_ROOT_PASSWORD="$$runner_secret_key" docker compose -f docker-compose.yml -p agentforge-artifact-integration --profile runner up --detach --wait minio; \
		AGENTFORGE_TEST_MINIO_ENDPOINT='127.0.0.1:29000' AGENTFORGE_TEST_MINIO_ACCESS_KEY="$$runner_access_key" AGENTFORGE_TEST_MINIO_SECRET_KEY="$$runner_secret_key" go test -count=1 -tags=artifactintegration ./services/platform-api/internal/adapters/artifactstore

test-controller:
	@$(MAKE) -C operator test
	@KUBEBUILDER_ASSETS="$$(operator/bin/setup-envtest use 1.36.0 --bin-dir "$(CURDIR)/operator/bin" -p path)" go test -count=1 -tags=controllerintegration ./services/platform-api/internal/application/handoff

test-controller-kind:
	@$(MAKE) -C operator test-kind

operator-manifests:
	@$(MAKE) -C operator manifests

operator-generate:
	@$(MAKE) -C operator generate

migrate:
	@if [[ -f .env ]]; then set -a; source .env; set +a; fi; go run ./services/platform-api/cmd/migrate

bootstrap-topics:
	@scripts/bootstrap-topics.sh

build-platform-api:
	@docker build \
		--build-arg BUILD_VERSION="$(BUILD_VERSION)" \
		--build-arg BUILD_COMMIT="$(BUILD_COMMIT)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--tag agentforge/platform-api:dev \
		--file services/platform-api/Dockerfile .

build-scheduler:
	@docker build \
		--build-arg BUILD_VERSION="$(BUILD_VERSION)" \
		--build-arg BUILD_COMMIT="$(BUILD_COMMIT)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--tag agentforge/scheduler:dev \
		--file services/platform-api/Scheduler.Dockerfile .

build-handoff:
	@docker build \
		--build-arg BUILD_VERSION="$(BUILD_VERSION)" \
		--build-arg BUILD_COMMIT="$(BUILD_COMMIT)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--tag agentforge/agentrun-handoff:dev \
		--file services/platform-api/Handoff.Dockerfile .

build-operator:
	@$(MAKE) -C operator docker-build IMG=agentforge/operator:dev

build-runner:
	@docker build \
		--build-arg BUILD_VERSION="$(BUILD_VERSION)" \
		--build-arg BUILD_COMMIT="$(BUILD_COMMIT)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--tag agentforge/runner:dev \
		--file agent-runner/Dockerfile .

check-all:
	@$(MAKE) check-tools format lint test verify-event-contracts test-integration test-events-integration test-runner-integration test-artifact-integration test-controller build-platform-api build-scheduler build-handoff build-operator build-runner verify

check-all-kind: check-all
	@$(MAKE) test-controller-kind

verify:
	@scripts/verify-repository.sh
