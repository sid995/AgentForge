# Platform API Service

## Responsibilities

- Authenticate requests and resolve tenant context.
- Authorize commands and queries.
- Create projects and agent runs.
- Accept cancellation, retry, deployment, and rollback commands.
- Return durable state and artifact metadata.
- Issue bounded artifact download capabilities only after tenant/run/metadata
  authorization; never return object-store credentials or raw object keys as
  metadata fields.

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
- Artifact storage configuration and credentials are server-owned. A missing or
  unavailable download store fails closed; it never falls back to a filesystem
  path or an unbounded redirect.
- Artifact payload deletion is a project-administrator-only HOT retention
  action. It removes the object before writing immutable PostgreSQL tombstone
  evidence; ARCHIVE automation remains an explicit future policy decision.

## Scaling

Stateless replicas behind a load balancer. Readiness fails when mandatory database access is unavailable. Redis failure falls back to PostgreSQL for supported reads.
