SHELL := /bin/bash

.DEFAULT_GOAL := help

.PHONY: help check-tools format lint test test-integration test-controller verify

help:
	@printf '%s\n' 'AgentForge development targets:'
	@printf '%s\n' '  check-tools       Check Phase 0 and Phase 1 prerequisites'
	@printf '%s\n' '  format            Format tracked Go source when it exists'
	@printf '%s\n' '  lint              Run Go linting (unavailable until Go source exists)'
	@printf '%s\n' '  test              Run unit tests (unavailable until Go source exists)'
	@printf '%s\n' '  test-integration  Run integration tests (unavailable until configured)'
	@printf '%s\n' '  test-controller   Run controller tests (unavailable until configured)'
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
	@if ! find . -type f -name '*.go' -not -path './.git/*' -print -quit | grep -q .; then \
		echo 'Go linting is unavailable: Phase 0 contains no Go source files.' >&2; \
		exit 1; \
	fi; \
	if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo 'Go linting requires golangci-lint v2; see docs/local-prerequisites.md.' >&2; \
		exit 1; \
	fi; \
	golangci-lint run ./...

test:
	@if ! find . -type f -name '*.go' -not -path './.git/*' -print -quit | grep -q .; then \
		echo 'Unit tests are unavailable: Phase 0 contains no Go source files.' >&2; \
		exit 1; \
	fi; \
	go test ./...

test-integration:
	@echo 'Integration tests are unavailable: Phase 0 has no service or dependency environment.' >&2
	@exit 1

test-controller:
	@echo 'Controller tests are unavailable: Phase 0 has no Kubernetes operator.' >&2
	@exit 1

verify:
	@scripts/verify-repository.sh
