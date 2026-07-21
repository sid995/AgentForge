# Specification Manifest

## Status policy

The supplied specifications have not completed the repository's approval
workflow. They are therefore **Draft** unless an accepted ADR or an explicit
approval record says otherwise. A Draft specification is still the governing
design input for the bounded task that names it; contradictions must be
surfaced, not silently resolved.

| Specification | Status | Owner component | Intended implementation | Intended tests |
|---|---|---|---|---|
| `00-overview/01-vision.md` | Draft | Product | N/A — product intent | N/A |
| `00-overview/02-scope.md` | Draft | Product | N/A — release scope | N/A |
| `00-overview/03-glossary.md` | Draft | Architecture | N/A — shared terminology | N/A |
| `01-product/01-requirements.md` | Draft | Product | Cross-cutting; see traceability | Cross-cutting; see traceability |
| `01-product/02-user-stories.md` | Draft | Product | Cross-cutting | Acceptance tests by feature |
| `01-product/03-non-functional-requirements.md` | Draft | Architecture | Cross-cutting | Security, resilience, and performance tests |
| `02-architecture/01-system-context.md` | Draft | Architecture | Repository-wide | Architecture review |
| `02-architecture/02-container-architecture.md` | Draft | Architecture | `cmd/`, `operator/`, `agent-runner/` | Integration and deployment tests |
| `02-architecture/03-quality-attributes.md` | Draft | Architecture | Cross-cutting | Resilience and security tests |
| `02-architecture/04-design-patterns.md` | Draft | Architecture | `internal/`, `operator/` | Unit and integration tests |
| `03-domain/01-domain-model.md` | Draft | Domain | `internal/domain/` | Domain unit tests |
| `03-domain/02-state-machines.md` | Draft | Domain | `internal/domain/` | Transition and concurrency tests |
| `03-domain/03-error-taxonomy.md` | Draft | Domain | `internal/domain/`, API adapters | Unit and contract tests |
| `04-services/01-platform-api.md` | Draft | Platform API | `cmd/platform-api/`, `internal/` | API unit and integration tests |
| `04-services/02-scheduler.md` | Draft | Scheduler | `cmd/scheduler/`, `internal/` | Scheduler unit and integration tests |
| `04-services/03-workflow-orchestrator.md` | Draft | Workflow orchestrator | `cmd/workflow-orchestrator/`, `internal/` | Workflow integration tests |
| `04-services/04-agent-operator.md` | Draft | Agent operator | `operator/` | Envtest and kind tests |
| `04-services/05-agent-runner.md` | Draft | Agent runner | `agent-runner/` | Runner container tests |
| `04-services/06-model-gateway.md` | Draft | Model gateway | `cmd/model-gateway/`, `internal/` | Provider contract tests |
| `04-services/07-build-service.md` | Draft | Build service | `cmd/build-service/`, `internal/` | Build integration tests |
| `04-services/08-deployment-service.md` | Draft | Deployment service | `cmd/deployment-service/`, `deploy/` | Git and Argo contract tests |
| `04-services/09-usage-audit-streaming.md` | Draft | Usage and audit | `internal/`, `cmd/` | Ledger and streaming tests |
| `05-apis/01-rest-api.md` | Draft | Platform API | `cmd/platform-api/` | API contract tests |
| `05-apis/02-api-examples.md` | Draft | Platform API | `cmd/platform-api/` | API contract tests |
| `05-apis/03-versioning-and-idempotency.md` | Draft | Platform API | `internal/`, API adapters | Idempotency and compatibility tests |
| `06-data/01-relational-schema.md` | Draft | Persistence | `db/migrations/`, `internal/adapters/` | Migration and integration tests |
| `06-data/02-indexing-and-querying.md` | Draft | Persistence | `db/migrations/`, `internal/adapters/` | Query-plan and integration tests |
| `06-data/03-retention-backup-recovery.md` | Draft | Operations | `db/`, `runbooks/` | Restore and recovery drills |
| `06-data/04-migrations.md` | Draft | Persistence | `db/migrations/`, `cmd/migrate/` | Migration and integration tests |
| `07-events/01-event-envelope.md` | Draft | Messaging | `internal/`, event schemas | Event contract tests |
| `07-events/02-topic-catalog.md` | Draft | Messaging | `internal/`, broker configuration | Event contract tests |
| `07-events/03-delivery-retry-dlq.md` | Draft | Messaging | `internal/`, outbox relay | Broker-failure and duplicate-delivery tests |
| `07-events/04-event-architecture.md` | Draft | Messaging | `internal/events/`, messaging adapters, outbox relay | Contract, PostgreSQL, and Redpanda integration tests |
| `08-kubernetes/01-crd-agent-run.md` | Draft | Agent operator | `operator/api/`, `operator/config/` | CRD schema and admission tests |
| `08-kubernetes/02-workload-resources.md` | Draft | Agent operator | `operator/` | Manifest security tests |
| `08-kubernetes/03-controller-behavior.md` | Draft | Agent operator | `operator/controllers/` | Envtest and kind tests |
| `09-security/01-threat-model.md` | Draft | Security | Cross-cutting | Security review and adversarial tests |
| `09-security/02-identity-rbac-secrets.md` | Draft | Security | API, operator, secret broker | Authorization and isolation tests |
| `09-security/03-sandbox-and-network.md` | Draft | Security | `operator/`, policies | Manifest and network-policy tests |
| `09-security/04-security-testing.md` | Draft | Security | `tests/`, CI | Security integration tests |
| `10-observability/01-telemetry-standards.md` | Draft | Observability | `internal/observability/` | Telemetry integration tests |
| `10-observability/02-slos-alerts-dashboards.md` | Draft | Observability | `observability/` | Alert and dashboard checks |
| `11-infrastructure/01-local-environment.md` | Draft | Platform engineering | `deploy/`, scripts, Makefile | Local-environment smoke tests |
| `11-infrastructure/02-cloud-and-terraform.md` | Draft | Platform engineering | `terraform/` | Terraform validation and integration tests |
| `11-infrastructure/03-gitops-layout.md` | Draft | Platform engineering | `deploy/argocd/` | GitOps contract tests |
| `12-testing/01-test-strategy.md` | Draft | Quality engineering | `tests/` | N/A — test policy |
| `12-testing/02-test-catalog.md` | Draft | Quality engineering | `tests/` | N/A — test inventory |
| `12-testing/03-load-chaos-performance.md` | Draft | Quality engineering | `tests/`, `runbooks/` | Load and chaos tests |
| `13-delivery/01-ci-cd.md` | Draft | Platform engineering | `.github/`, `deploy/` | CI workflow checks |
| `13-delivery/02-release-definition.md` | Draft | Platform engineering | `deploy/`, release automation | Release verification |
| `14-operations/01-runbooks.md` | Draft | Operations | `runbooks/` | Incident exercises |
| `14-operations/02-incident-management.md` | Draft | Operations | `runbooks/` | Incident simulations |
| `14-operations/03-capacity-cost.md` | Draft | Operations | Usage ledger and observability | Ledger and capacity tests |
| `15-llm-development/01-llm-engineering-workflow.md` | Draft | Engineering productivity | N/A — engineering process | Process review |
| `15-llm-development/02-context-packs.md` | Draft | Engineering productivity | N/A — engineering process | Process review |
| `15-llm-development/03-prompt-library.md` | Draft | Engineering productivity | N/A — engineering process | Process review |
| `15-llm-development/04-agent-roles-and-gates.md` | Draft | Engineering productivity | N/A — engineering process | Process review |
| `15-llm-development/05-codex-execution-playbook.md` | Draft | Engineering productivity | N/A — engineering process | Repository validation |
| `16-implementation/01-repository-layout.md` | Draft | Architecture | Repository-wide | Repository validation |
| `16-implementation/02-coding-standards.md` | Draft | Architecture | Repository-wide | Lint and test review |
| `16-implementation/03-milestones.md` | Draft | Product | Repository-wide | Phase-gate audits |
| `16-implementation/04-definition-of-done.md` | Draft | Architecture | Repository-wide | Phase-gate audits |
| `17-decisions/README.md` | Draft | Architecture | `docs/adr/` | ADR review |
| `18-templates/adr-template.md` | Draft | Architecture | `docs/adr/` | N/A — template |
| `18-templates/llm-task-template.md` | Draft | Engineering productivity | N/A — template | N/A — template |
| `18-templates/runbook-template.md` | Draft | Operations | `runbooks/` | N/A — template |
| `18-templates/service-spec-template.md` | Draft | Architecture | `specs/` | N/A — template |

## Repository controls

| Control | Purpose |
|---|---|
| `specs/README.md` | Specification entry point |
| `PROJECT-STATUS.md` | Current implementation state |
| `TRACEABILITY.md` | Requirement-to-code-and-test evidence |
| `15-llm-development/audits/` | Bootstrap and phase-completion audits |

## Maintenance rules

- Update the status, owner, implementation path, and test path whenever a
  specification is approved or implementation evidence changes.
- Mark a requirement **Implemented** or **Verified** only in
  `TRACEABILITY.md` and only with code and test evidence.
- Link accepted ADRs, migrations, public contracts, and generated artifacts
  from the governing specification when they are introduced.
