# System Context

```mermaid
flowchart LR
  User[Developer / Operator] --> Edge[Cloudflare / API Gateway]
  Edge --> Control[AgentForge Control Plane]
  Control --> Exec[Kubernetes Execution Plane]
  Exec --> Models[Model Providers]
  Exec --> Git[Source Repositories]
  Exec --> Store[Object Storage]
  Control --> Registry[Container Registry]
  Control --> GitOps[GitOps Repository]
  GitOps --> Argo[Argo CD]
  Argo --> Apps[Application Plane]
  Control --> Obs[Observability Stack]
```

## Trust boundaries

- Public edge to control plane.
- Control plane to execution clusters.
- Execution workloads to approved external systems.
- Control plane to deployment repository and application clusters.
- Tenant boundary across every persistence and authorization layer.
