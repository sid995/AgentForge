# Implementation Repository Layout

```text
agentforge/
  cmd/
    platform-api/
    scheduler/
    workflow-orchestrator/
    model-gateway/
    build-service/
    deployment-service/
  internal/
    domain/
    application/
    ports/
    adapters/
    observability/
  operator/
    api/v1alpha1/
    controllers/
    config/
  agent-runner/
  web/
  db/migrations/
  deploy/helm/
  deploy/kustomize/
  deploy/argocd/
  terraform/
  observability/
  policies/
  tests/
  runbooks/
  docs/adr/
```

Start as a modular control-plane repository. Extract independently deployed services only where scaling, isolation, ownership, or release cadence justifies it.
