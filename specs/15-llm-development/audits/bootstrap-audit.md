# Phase 0 Bootstrap Audit

**Date:** 2026-07-21
**Scope:** supplied repository and specification bundle only; no production code was evaluated or created.

## Repository inventory

The repository contains 73 tracked Markdown files and one bootstrap commit. It
contains no application code, build harness, tests, CI configuration, database
migrations, deployment manifests, or generated artifacts. The working tree was
clean at audit start.

| Area | Available evidence | Assessment |
|---|---|---|
| Repository controls | Root `README.md`, `START-HERE.md`, `AGENTS.md`, and `BUNDLE-CONTENTS.md` | Present |
| Product through operations | `specs/00-overview/` through `specs/14-operations/` | Present in the canonical supplied hierarchy |
| LLM workflow and implementation guidance | `specs/15-llm-development/` and `specs/16-implementation/` | Present, including the ordered execution playbook |
| Decision and task templates | `specs/17-decisions/` and `specs/18-templates/` | Present as templates and decision backlog only |
| Status and traceability | `PROJECT-STATUS.md` and `TRACEABILITY.md` | Present and accurately report no implementation |
| Development harness | Makefile, scripts, editor configuration, GitHub Actions | Missing; explicitly deferred to Prompt 0.4 |
| Production implementation | Go workspace, services, operator, database, integrations | Missing; expected at Phase 0 |

All Markdown links currently present in repository controls resolve. The
manifest now indexes the execution playbook alongside the supplied
specifications.

## Missing or stale references

The execution playbook contained forward-looking paths that did not exist in
the supplied hierarchy. This task aligns its current references with the
supplied hierarchy; future output paths are now under `15-llm-development/`.

| Source | Referenced path or area | Current equivalent or disposition |
|---|---|---|
| Execution-playbook Phase 0 and audit paths | obsolete `12-llm-development` location | Corrected to `15-llm-development/audits/` |
| Execution-playbook glossary reference | obsolete `specs/GLOSSARY.md` path | Corrected to `00-overview/03-glossary.md` |
| Execution-playbook Phase 1–5 reading lists | normalized services, observability, testing, roadmap, data, domain, security, and event paths | Corrected to the supplied specification files |
| Execution-playbook Phase 19 review output | nonexistent `11-operations` location | Corrected to `14-operations/operational-readiness-review.md` |

## Contradictions and decisions required

1. **Technology choices awaiting ADRs.** The decision backlog identifies Go,
   Kafka, Kubernetes Jobs plus an AgentRun operator, rootless BuildKit, Argo
   CD, object storage, and a single cloud provider, but contains no accepted
   ADRs. `00-overview/02-scope.md` also permits Kafka *or* Redpanda. These
   choices must be recorded before their respective implementation phases.
2. **Security delivery order.** The playbook schedules production OIDC and
   authorization in Phase 12, while the API is introduced in Phase 1. The
   Phase 1 plan must define a bounded development-identity approach and must
   not accept client-supplied tenant identity or weaken later tenant-isolation
   controls.
3. **Migration contract detail.** The data specification requires forward
   migrations and documents recovery goals, but the normalized playbook cites a
   dedicated migrations specification that is not supplied. Add that contract
   during specification normalization before persistence implementation.

## Proposed canonical repository layout

The supplied `00-overview` through `18-templates` hierarchy is canonical.
Product code should adopt the modular control-plane layout already specified in
`16-implementation/01-repository-layout.md`:

```text
agentforge/
  cmd/                 # independently executable control-plane binaries
  internal/            # domain, application, ports, adapters, observability
  operator/            # AgentRun API, controllers, and controller config
  agent-runner/        # isolated workload runtime
  db/migrations/       # forward database migrations
  deploy/              # Helm, Kustomize, and Argo CD desired state
  terraform/           # cloud infrastructure
  tests/               # cross-component integration and end-to-end tests
  docs/adr/            # accepted architecture decisions
  specs/               # approved specifications, status, and traceability
```

Do not create these production directories until the relevant bounded phase
starts; Phase 0 should add only the documented development harness.

## Root AGENTS.md responsibilities

The existing root `AGENTS.md` covers the required responsibilities: source
precedence, task workflow, architecture invariants, tenant scoping, security
prohibitions, contract and migration policy, stable validation targets,
documentation updates, and completion reporting. Keep it as the single root
instruction file during bootstrap. Add nested instructions only when a real
major boundary exists and needs rules that differ from the root.

## First implementation dependency order

1. Complete Phase 0 with the development harness and repository verification.
2. Establish the Go workspace and Platform API foundation, including health,
   request correlation, bounded configuration, and a documented development
   identity boundary.
3. Add PostgreSQL migrations, tenants/projects, tenant-scoped repositories,
   and row-level security before tenant-owned commands.
4. Implement the AgentRun domain, optimistic transitions, idempotent create
   and cancellation commands, and transactional audit/outbox writes.
5. Add the transactional outbox, versioned events, broker adapter, and
   idempotent consumer foundation.
6. Add scheduling and deterministic AgentRun CR creation, then the CRD and
   idempotent operator reconciliation.
7. Add the restricted runner and artifact flow before model, build, and
   deployment integrations.

## Recommended next bounded task

Execute Playbook Prompt 0.4 to create the development harness without
application code.
