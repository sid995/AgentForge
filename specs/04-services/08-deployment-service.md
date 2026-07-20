# Deployment Service

## Responsibilities

- Validate release and environment policy.
- Collect required approvals.
- Render deployment manifests.
- Commit desired state to GitOps repository.
- Track Argo CD sync, health, and resource tree.
- Implement rollback and promotion.

## Environment defaults

- Development: automatic deployment permitted.
- Staging: optional approval.
- Production: mandatory human approval and successful policy checks.

## Concurrency

Serialize deployment commands by `project_id + environment`. Use optimistic locking and deterministic Git branch or commit semantics to prevent competing promotions.

## Rollback

Rollback creates a new auditable desired-state commit pointing to a prior immutable digest. It does not mutate history.
