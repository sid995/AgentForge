# ADR-004: Kubernetes AgentRun custom resource and controller-runtime operator

- Status: Accepted
- Date: 2026-07-22
- Owners: Agent Operator, Platform engineering, Security

## Context

Scheduled AgentForge attempts need a durable, cluster-local desired state that
can survive Scheduler and controller restarts. Direct Job creation by the
Scheduler would mix placement with workload convergence, make partial failures
harder to repair, and bypass a single enforcement point for workload security.

## Decision

Define the namespaced `execution.agentforge.dev/v1alpha1` `AgentRun` custom
resource and reconcile it with a dedicated controller-runtime Operator. The
Scheduler intent may be projected by a separate idempotent handoff component
that creates or compares deterministic AgentRun resources but never creates
Jobs; the Scheduler process itself remains database/event-only. The Operator
is the sole owner of execution ServiceAccounts,
configuration, workspace claims, NetworkPolicies, and Jobs.

The project is generated with Kubebuilder v4 and pins controller-runtime,
Kubernetes libraries, controller-gen, kustomize, envtest, and lint tooling.
Generated CRDs, RBAC, and deepcopy code are checked in and reproducible from
source markers. The manager uses leader election, structured logs, health and
readiness probes, authenticated metrics, least-privilege RBAC, and a non-root
image. Reconciliation behavior is added only in later Phase 6 sub-phases.

## Alternatives considered

- Scheduler creates Jobs directly: rejected because scheduling delivery and
  Kubernetes convergence would share failure and ownership boundaries.
- Store desired workload only in PostgreSQL: rejected because Kubernetes needs
  a declarative object that controllers can observe and reconcile locally.
- Deployments or Pods as the execution primitive: rejected because one attempt
  is bounded batch work with terminal completion semantics.
- A custom controller without controller-runtime: rejected because it would
  duplicate mature scheme, cache, leader-election, metrics, probe, and envtest
  foundations without an approved operational benefit.

## Consequences

The CRD becomes a versioned public contract requiring compatibility and future
conversion planning. Kubernetes status is observed cluster state; PostgreSQL
remains the authoritative platform workflow store, joined through explicit
events and idempotent handoff. Controller tests require envtest, while Pod/Job
behavior and restart recovery require a pinned real-cluster kind gate.

## Revisit conditions

Revisit if Kubernetes is no longer the execution substrate, Jobs cannot model
the required workload lifecycle, or measured controller-runtime behavior fails
the reconciliation, security, or availability requirements.
