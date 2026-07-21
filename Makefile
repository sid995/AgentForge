SHELL := /bin/bash

.DEFAULT_GOAL := help

GOLANGCI_LINT_IMAGE := golangci/golangci-lint:v2.9.0
BUILD_VERSION ?= development
BUILD_COMMIT ?= unknown
BUILD_TIME ?= unknown

.PHONY: help check-tools format lint test test-integration test-controller migrate build-platform-api verify

help:
	@printf '%s\n' 'AgentForge development targets:'
	@printf '%s\n' '  check-tools       Check Phase 0 through Phase 3 prerequisites'
	@printf '%s\n' '  format            Format tracked Go source'
	@printf '%s\n' '  lint              Run Platform API static analysis'
	@printf '%s\n' '  test              Run Platform API unit tests'
	@printf '%s\n' '  test-integration  Run PostgreSQL integration tests in Docker Compose'
	@printf '%s\n' '  test-controller   Run controller tests (unavailable until configured)'
	@printf '%s\n' '  migrate           Apply checked-in PostgreSQL migrations'
	@printf '%s\n' '  build-platform-api Build the Platform API container image (BUILD_VERSION, BUILD_COMMIT, BUILD_TIME are supported)'
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

test:
	@go test ./services/platform-api/...

test-integration:
	@set -euo pipefail; \
		cleanup() { docker compose -f docker-compose.yml -p agentforge-phase2-test down --volumes --remove-orphans; }; \
		trap cleanup EXIT; \
		POSTGRES_DB=agentforge POSTGRES_USER=agentforge_migrator POSTGRES_PASSWORD=agentforge-migrator POSTGRES_APP_PASSWORD=agentforge-app POSTGRES_HOST_PORT=15432 docker compose -f docker-compose.yml -p agentforge-phase2-test up --detach --wait; \
		AGENTFORGE_DATABASE_URL='postgres://agentforge_migrator:agentforge-migrator@127.0.0.1:15432/agentforge?sslmode=disable' go run ./services/platform-api/cmd/migrate; \
		AGENTFORGE_TEST_DATABASE_URL='postgres://agentforge_migrator:agentforge-migrator@127.0.0.1:15432/agentforge?sslmode=disable' \
		AGENTFORGE_TEST_APP_DATABASE_URL='postgres://agentforge_app:agentforge-app@127.0.0.1:15432/agentforge?sslmode=disable' \
		go test -count=1 -tags=integration ./services/platform-api/...

test-controller:
	@echo 'Controller tests are unavailable: Phase 1 has no Kubernetes operator.' >&2
	@exit 1

migrate:
	@go run ./services/platform-api/cmd/migrate

build-platform-api:
	@docker build \
		--build-arg BUILD_VERSION="$(BUILD_VERSION)" \
		--build-arg BUILD_COMMIT="$(BUILD_COMMIT)" \
		--build-arg BUILD_TIME="$(BUILD_TIME)" \
		--tag agentforge/platform-api:dev \
		--file services/platform-api/Dockerfile .

verify:
	@scripts/verify-repository.sh
