# Phase 6 Completion Audit: Kubernetes AgentRun and Operator

**Audited:** 2026-07-23

## Scope reviewed

- Phase 6 plan, ADR-004, Agent Operator, CRD, workload-resource,
  controller-behavior, sandbox/network, event, data, observability, and testing
  specifications
- Operator bootstrap and CRD contract; reconciliation, secure resource
  construction, prerequisites, lifecycle, cancellation, retries, and retained
  workspace cleanup
- The transactional scheduled-intent handoff, registered-cluster selection,
  tenant authorization, Kubernetes idempotency, retry/DLQ behavior, root
  Compose packaging, resource limits, and local environment contract
- Unit, PostgreSQL, Kafka, envtest, kind, race, static, repository, Compose,
  contract, and image gates

The supplied prompt used the obsolete `12-llm-development` audit prefix. The
canonical manifest path is `15-llm-development/audits/`.

## Phase gate evidence

| Requirement | Evidence | Result |
|---|---|---|
| Creating an AgentRun creates a secure Job | The structural CRD and pure builders enforce trusted placement, resource bounds, restricted Pod/container security, optional runtime class, read-only references, and network policy; prerequisite reconciliation creates the Job only after references and storage are ready | Passed |
| Reconciliation is idempotent | Deterministic names, ownership validation, non-forced server-side apply, semantic status comparison, generation filtering, terminal stability, and exact existing-object comparison cover repeated Operator and handoff reconciliation | Passed |
| Status reflects observed workload state | Job/Pod observation projects pending, scheduled, container-creating, running, succeeded, and normalized failed states with timestamps, bounded attempt history, mandatory result evidence, and conflict-safe status writes | Passed |
| Cancellation, retry, and cleanup are bounded | Cancellation has restart-safe graceful/forced deadlines; retries intersect desired and platform policy with deterministic bounded backoff and fresh attempt Jobs; retained-PVC finalization is idempotent and has a diagnostic escalation deadline | Passed |
| Scheduled intent reaches the selected cluster safely | A transactionally paired immutable intent row, explicit one-to-one cluster-context mapping, tenant allowlist, deterministic Namespace/AgentRun, durable marker, exact comparison, and retry/DLQ routing bridge the unchanged `agent-run.scheduled.v1` fact | Passed |
| Security boundaries are preserved | The Scheduler has no Kubernetes client; the handoff creates only Namespace and AgentRun; the Operator alone creates execution resources and owns status; Secret existence uses uncached metadata-only get access; no privileged workload, host socket, `hostPath`, or raw secret copying is introduced | Passed |
| Documentation and traceability are current | Status, traceability, component/data/event/Kubernetes/security/testing/local-environment specs, Phase plan, README, `.env.example`, root Compose, and this audit point to code and test evidence | Passed |

## Severity-ranked specialist review

| Severity | Finding | Resolution |
|---|---|---|
| High | Sleeping on a delayed retry record in the single consumer loop could block newly scheduled primary events | Resolved by running an independent Kafka group member for each primary and retry topic; focused tests prove a delayed tier cannot head-of-line block another tier |
| Medium | Cached Secret reads required cluster-wide list/watch permissions and could retain Secret payloads in the controller cache | Resolved with the uncached API reader, `PartialObjectMetadata`, and get-only Secret RBAC; StorageClass lookup is likewise uncached and get-only |
| Medium | Handoff work had no per-record deadline, and lazy kube-context resolution could defer invalid or ambiguous mappings until traffic arrived | Resolved with a bounded per-event timeout and startup validation that every configured cluster maps to one known, unique kubeconfig context |

No unresolved high-severity finding remains.

## Concrete failure interleavings

### Scheduling transaction and publication

The scheduling transaction writes assignment, reservations, run/attempt state,
the immutable handoff intent, and the outbox event atomically. A crash before
commit leaves none of them; a crash after commit leaves the complete handoff
input addressable by event ID. The outbox relay may publish more than once, but
cannot publish a scheduled fact whose companion intent is absent.

### Crash after Kubernetes effect

The consumer validates the event, intent, registered cluster, tenant
authorization, and desired CR before creating anything. If it crashes after
the Kubernetes create and before the processed marker commits, replay reads
the defaulted object from the real API and requires an exact semantic match.
It then records the durable marker without creating another object. A changed
or foreign-owned object is a permanent conflict, never silently adopted.

### Duplicate replicas

Two replicas can receive the same at-least-once fact. Deterministic names and
Kubernetes create semantics select one creator; the other performs exact
comparison. The PostgreSQL processed-event transaction admits one marker.
Namespace and AgentRun are the only handoff effects. The handoff never creates
a Job or writes the status subresource.

### Retry and DLQ publication

Transient failures publish the sanitized retry envelope before acknowledging
the source record. Permanent failures publish the sanitized DLQ envelope
before acknowledgement. If publication succeeds and acknowledgement is lost,
Kafka may redeliver; deterministic Kubernetes comparison and the durable
marker make the replay safe. Primary and each retry tier use distinct group
members, so a delayed retry does not block fresh scheduled work or another
tier.

### Cluster and tenant isolation

The event's cluster and tenant must exactly match the immutable database
intent. The cluster must be registered, enabled, and mapped to one known,
unique kubeconfig context; tenant authorization must include that cluster.
Unknown, disabled, ambiguous, unauthorized, or mismatched intent is rejected
before a Kubernetes write. Kubeconfig contexts are documented as
least-privilege handoff identities, never platform or cloud administrator
credentials.

## Controller and Kubernetes review

- Desired fields are immutable except for one-way cancellation; CRD defaults
  and cross-field validation run in Kubernetes.
- The cache holds owned/watchable workload metadata, but Secret and
  StorageClass existence checks use uncached, get-only reads.
- Owner references govern disposable namespaced children. The finalizer exists
  only for retained workspaces and removes itself after a bounded,
  diagnostically visible handoff attempt.
- Restricted security contexts, non-root execution, no privilege escalation,
  dropped capabilities, read-only root filesystem, seccomp, resource limits,
  active deadline, and selected network policy are builder invariants.
- Cancellation filters forced Pod deletion by exact owner UID. Retry commits
  status before creating fresh attempt-qualified resources. Terminal state is
  stable and a deleted observed Job is not recreated.
- The kind gate exercises real Job lifecycle, two distinct retry Jobs, and
  retained PVC survival. The envtest suite exercises schema, API behavior,
  reconciliation ordering, conflicts, and deletion paths.

## Validation record

The final command results below were run after all Phase 6 implementation and
specialist-review corrections.

| Command | Result |
|---|---|
| `make check-tools` | Passed; required tools present and optional host tools reported honestly |
| `make format` | Passed |
| `make lint` | Passed for the Platform API and Operator modules |
| `make test` | Passed |
| `make verify-event-contracts` | Passed for schemas, examples, immutable baselines, producers, and consumer fixtures |
| `go test -race ./services/platform-api/...` | Passed |
| `go vet ./services/platform-api/...` | Passed |
| `make verify` | Passed |
| `docker compose config --quiet` | Passed; one root Compose file with CPU and memory ceilings for every service |
| `COMPOSE_PULL_POLICY=never make test-integration` | Passed against fresh PostgreSQL migrations 1 through 11, including the handoff role and failure-window paths |
| `COMPOSE_PULL_POLICY=never make test-events-integration` | Passed against isolated pinned Redpanda and bootstrapped topics |
| `make test-controller` | Passed against Kubernetes 1.36 envtest; controller package coverage is 80.4% |
| `make test-controller-kind` | Passed on kind 0.32.0/Kubernetes 1.36.1 for lifecycle, retry, and retained PVC behavior |
| `KUBEBUILDER_ASSETS=<1.36 assets> GOWORK=off go test -race ./...` in `operator/` | Passed |
| `make build-platform-api build-scheduler build-handoff build-operator` | Passed; all four images declare a non-root UID |

Envtest does not schedule Pods or enforce runtime/network behavior. The pinned
kind gate supplies the real API/controller/Job/PVC evidence that envtest
cannot, while manifest security and mutation tests cover the policy contract.

## Remaining boundaries and follow-up work

| Severity | Item | Owner phase |
|---|---|---|
| Medium | Observed Kubernetes lifecycle is not yet projected back into PostgreSQL. The Phase 6 Operator preserves status-subresource ownership and does not invent the later runner/lifecycle event contract. | Agent Runner and lifecycle integration |
| Medium | Production deployment, scoped multi-cluster kubeconfigs, Kafka ACL/TLS, high availability, and GitOps manifests are not part of the local handoff packaging. | Infrastructure, security, and delivery phases |
| Medium | Phase 3.3 create/get/list API, Phase 3.4 cancellation command, and Phase 3.5 retry command remain absent; their HTTP/authentication and command-race behavior is not claimed here. | Resume Phase 3 |
| Low | The local kind CNI does not prove production NetworkPolicy enforcement or production admission/runtime sandbox configuration. | Security and production cluster validation |
| Low | Local PostgreSQL and Redpanda use development-only credentials, resource ceilings, and topology. | Infrastructure and operations |

## Conclusion

Phase 6 satisfies its Kubernetes Operator gate. Scheduled intent is
transactionally handed to the explicitly authorized cluster, AgentRun
reconciliation creates secure deterministic workloads, observed status,
cancellation, retries, and retained cleanup are idempotent and bounded, and
the real-Kubernetes gate passes. This audit does not claim a production
multi-cluster deployment, Runner implementation, or PostgreSQL lifecycle
projection.
