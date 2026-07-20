# Context Packs

Create task-specific context packs instead of sending the entire repository.

## Standard pack

- Objective and acceptance criteria.
- Relevant specification files.
- Current interfaces and schemas.
- Adjacent implementation files.
- Existing tests and failure output.
- Constraints and explicit non-goals.

## Service feature pack

Include service spec, domain state machine, API/event contract, data tables, authorization rule, telemetry convention, and test requirements.

## Kubernetes pack

Include CRD spec, controller behavior, workload security, manifests, controller tests, and expected Kubernetes version.

## Incident pack

Include symptoms, recent changes, logs, metrics, trace excerpts, topology, and runbook. Ask for ranked hypotheses and discriminating tests, not immediate random edits.
