# AgentForge Development Specification

AgentForge is a production-style infrastructure platform for running isolated AI coding agents, validating generated applications, building immutable artifacts, and deploying them through GitOps.

This repository is the implementation contract for humans and LLM coding agents. It separates product intent, architecture, service boundaries, data contracts, infrastructure, security, testing, operations, and delivery rules so implementation work can be delegated without sacrificing coherence.

## Specification principles

1. PostgreSQL is the transactional source of truth.
2. Long-running work is asynchronous and event-driven.
3. Agent code and generated code are always untrusted.
4. Kubernetes controllers reconcile desired and actual state.
5. Every state-changing operation is idempotent and auditable.
6. Git is the source of deployment state.
7. Large artifacts live in object storage, not relational rows.
8. LLM output is treated as an untrusted proposal requiring validation.

## Reading order

1. `00-overview/`
2. `01-product/`
3. `02-architecture/`
4. `03-domain/`
5. `04-services/`
6. `05-apis/`, `06-data/`, and `07-events/`
7. `08-kubernetes/` through `14-operations/`
8. `15-llm-development/` and `16-implementation/`

## Definition of complete

A feature is complete only when its domain rules, API or event contracts, persistence, authorization, observability, tests, deployment configuration, runbook impact, and documentation are implemented.
