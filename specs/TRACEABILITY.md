# Requirements Traceability

This file maps approved requirements to implementation and test evidence. It must never claim implementation that does not exist.

## Status values

- **Specified**: documented but not implemented
- **Planned**: assigned to a milestone
- **Implemented**: code exists
- **Verified**: required tests pass
- **Deferred**: intentionally postponed with rationale

| Requirement | Specification | Intended implementation | Intended tests | Status |
|---|---|---|---|---|
| FR-PLT-001 Platform health endpoints | `04-services/01-platform-api.md` | `services/platform-api/` | `services/platform-api/**/*_test.go` | Specified |
| FR-RUN-001 Create agent run | `01-product/01-requirements.md` | `services/platform-api/internal/runs/` | `tests/platform-api/` | Specified |
| FR-RUN-002 Track run lifecycle | `03-domain/02-state-machines.md` | `internal/runs/` | domain and integration tests | Specified |
| FR-SCH-001 Claim queued runs safely | `04-services/02-scheduler.md` | `services/scheduler/` | `tests/scheduler/` | Specified |
| FR-OPR-001 Reconcile AgentRun CR | `08-kubernetes/03-controller-behavior.md` | `operator/controllers/` | envtest and kind tests | Specified |
| FR-EXE-001 Run isolated agent workload | `04-services/05-agent-runner.md` | `agent-runner/` | runner container tests | Specified |
| FR-BLD-001 Build immutable image | `04-services/07-build-service.md` | `services/build-service/` | build integration tests | Specified |
| FR-DEP-001 Deploy through GitOps | `04-services/08-deployment-service.md` | `services/deployment-service/` | Git and Argo contract tests | Specified |
| SEC-TEN-001 Tenant-scoped data access | `09-security/02-identity-rbac-secrets.md` | repositories and RLS | cross-tenant integration tests | Specified |
| SEC-EXE-001 Non-privileged agent workloads | `09-security/03-sandbox-and-network.md` | operator workload builder | manifest security tests | Specified |
| REL-EVT-001 Transactional event publication | `07-events/03-delivery-retry-dlq.md` | outbox relay | broker-failure integration tests | Specified |
| REL-EVT-002 Idempotent event consumption | `07-events/03-delivery-retry-dlq.md` | consumer foundation | duplicate-delivery tests | Specified |
| OBS-001 End-to-end correlated telemetry | `10-observability/01-telemetry-standards.md` | shared telemetry packages | trace integration test | Specified |
| COST-001 Per-run usage attribution | `14-operations/03-capacity-cost.md` | usage ledger | ledger idempotency tests | Specified |

## Maintenance rules

- Add stable requirement identifiers during the bootstrap normalization task.
- Update implementation and test paths when code is created.
- Mark **Implemented** only when code exists.
- Mark **Verified** only after the required validation passes.
- Link migrations, contracts, and ADRs when they govern the requirement.
