#!/usr/bin/env bash
set -euo pipefail

required_tools=(git make bash awk find go docker kubectl)
optional_tools=(helm kind terraform golangci-lint shellcheck)
missing=0

for tool in "${required_tools[@]}"; do
  if command -v "${tool}" >/dev/null 2>&1; then
    printf 'required: %s (%s)\n' "${tool}" "$(command -v "${tool}")"
  else
    printf 'missing required tool: %s\n' "${tool}" >&2
    missing=1
  fi
done

for tool in "${optional_tools[@]}"; do
  if command -v "${tool}" >/dev/null 2>&1; then
    printf 'optional: %s (%s)\n' "${tool}" "$(command -v "${tool}")"
  else
    printf 'optional (not required through Phase 6.4): %s unavailable\n' "${tool}"
  fi
done

if [[ "${missing}" -ne 0 ]]; then
  printf '%s\n' 'Install the missing required tools described in docs/local-prerequisites.md.' >&2
  exit 1
fi
