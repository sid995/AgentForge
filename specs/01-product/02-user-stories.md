# User Stories

## Run management

- As a developer, I can submit a task and immediately receive a run identifier.
- As a developer, I can follow live progress without keeping the original HTTP request open.
- As a developer, I can cancel a run and know whether termination completed.
- As a developer, I can retry a failed run while preserving prior attempts.

## Deployment

- As a developer, I can deploy successful output to development automatically.
- As a project administrator, I can require approval for staging and production.
- As an operator, I can roll back to a known image digest.

## Governance

- As a project administrator, I can set per-run and daily budgets.
- As a security auditor, I can inspect who accessed secrets or approved deployments.
- As an operator, I can identify noisy tenants and enforce quotas.
