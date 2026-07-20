# GitOps Layout

Use separate application source and environment repositories.

```text
environments/
  dev/tenant/project/
  staging/tenant/project/
  production/tenant/project/
```

Each release references image digest, configuration version, resource policy, and provenance. Argo CD owns reconciliation. Direct production mutation is restricted to audited emergency procedures.

Use ApplicationSet when project count justifies it. Define sync waves for namespace, policy, configuration, database migration Jobs, and application rollout.
