#!/usr/bin/env bash
set -euo pipefail

required_files=(
  AGENTS.md
  README.md
  START-HERE.md
  Makefile
  .editorconfig
  .gitignore
  .gitattributes
  .golangci.yml
  docs/local-prerequisites.md
  specs/README.md
  specs/SPEC-MANIFEST.md
  specs/PROJECT-STATUS.md
  specs/TRACEABILITY.md
  specs/15-llm-development/05-codex-execution-playbook.md
  specs/15-llm-development/audits/bootstrap-audit.md
)

for path in "${required_files[@]}"; do
  if [[ ! -f "${path}" ]]; then
    printf 'missing required repository file: %s\n' "${path}" >&2
    exit 1
  fi
done

if ! git diff --check; then
  printf '%s\n' 'repository has whitespace errors' >&2
  exit 1
fi

while IFS= read -r target; do
  [[ -z "${target}" ]] && continue
  if [[ ! -e "${target}" ]]; then
    printf 'README.md references a missing local path: %s\n' "${target}" >&2
    exit 1
  fi
done < <(grep -oE '\]\(([^)#]+)' README.md | sed -E 's/^\]\(//')

manifest_paths=$(grep -oE '`[0-9]{2}-[^`]+\.md`' specs/SPEC-MANIFEST.md | tr -d '`' | sort -u)
while IFS= read -r specification; do
  [[ -z "${specification}" ]] && continue
  relative_path=${specification#specs/}
  if ! grep -Fxq "${relative_path}" <<<"${manifest_paths}"; then
    printf 'specification missing from manifest: %s\n' "${specification}" >&2
    exit 1
  fi
done < <(find specs -mindepth 2 -type f -name '*.md' -not -path '*/audits/*' -not -path '*/plans/*' | sort)

if [[ -d db/migrations ]]; then
  previous_version=''
  while IFS= read -r up_migration; do
    migration_name=$(basename "${up_migration}")
    migration_version=${migration_name%%_*}
    down_migration="${up_migration%.up.sql}.down.sql"
    if [[ ! "${migration_version}" =~ ^[0-9]{6}$ ]]; then
      printf 'migration version must use six digits: %s\n' "${up_migration}" >&2
      exit 1
    fi
    if [[ ! -f "${down_migration}" ]]; then
      printf 'migration is missing a down file: %s\n' "${up_migration}" >&2
      exit 1
    fi
    if [[ -n "${previous_version}" && "${migration_version}" == "${previous_version}" ]]; then
      printf 'duplicate migration version: %s\n' "${migration_version}" >&2
      exit 1
    fi
    previous_version="${migration_version}"
  done < <(find db/migrations -maxdepth 1 -type f -name '*.up.sql' | sort)

  while IFS= read -r down_migration; do
    up_migration="${down_migration%.down.sql}.up.sql"
    if [[ ! -f "${up_migration}" ]]; then
      printf 'migration down file has no up file: %s\n' "${down_migration}" >&2
      exit 1
    fi
  done < <(find db/migrations -maxdepth 1 -type f -name '*.down.sql' | sort)
fi

printf '%s\n' 'repository verification passed'
