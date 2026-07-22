# Required Test Catalog

## Run lifecycle

- Duplicate create command.
- Invalid transition.
- cancellation races completion.
- timeout during model call.
- scheduler crash before and after CR creation.
- operator restart during active Job.
- retryable and terminal attempt failure.

## Messaging

- duplicate delivery.
- out-of-order aggregate version.
- publish failure after transaction.
- consumer crash after commit before acknowledgment.
- DLQ and authorized replay.
- canonical examples and typed producer output against machine-readable schemas.
- immutable-major required-field and JSON-type compatibility baselines.
- supported old consumer fixtures against current schemas and runtime decoders.
- retry-attempt progression and publish-before-acknowledgment against Redpanda.

## Isolation

- cross-tenant API and repository access.
- RLS enforcement.
- object-storage prefix isolation.
- Kubernetes namespace and egress isolation.

## Deployment

- competing promotions.
- invalid manifest.
- Argo degraded rollout.
- rollback to prior digest.
