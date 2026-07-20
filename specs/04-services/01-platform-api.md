# Platform API Service

## Responsibilities

- Authenticate requests and resolve tenant context.
- Authorize commands and queries.
- Create projects and agent runs.
- Accept cancellation, retry, deployment, and rollback commands.
- Return durable state and artifact metadata.

## Architecture

- HTTP adapter.
- Application command/query handlers.
- Domain aggregates and policies.
- PostgreSQL repositories.
- Outbox writer.
- Redis cache adapter.

## Critical rules

- Require an idempotency key for run creation and deployment commands.
- Never accept a client-provided status mutation.
- Never trust a tenant ID from request payload.
- Write state and outbox event in the same database transaction.
- Redact prompts, source, secrets, and authorization headers from logs.

## Scaling

Stateless replicas behind a load balancer. Readiness fails when mandatory database access is unavailable. Redis failure falls back to PostgreSQL for supported reads.
