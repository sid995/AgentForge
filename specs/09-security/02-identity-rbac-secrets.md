# Identity, RBAC, and Secrets

## Human identity

OIDC authentication with short-lived tokens. Membership maps users to tenant and project roles.

## Workload identity

Services and Jobs use cloud workload identity or short-lived internal credentials. Static cloud keys are forbidden.

## Authorization

Enforce permission checks at API, service, repository, RLS, Kubernetes RBAC, object-storage prefix, and consumer boundaries.

## Secrets

Store secrets in a cloud secret manager or Vault. Database rows hold references only. The agent accesses approved capabilities through brokers or scoped credentials. Every secret access is audited.
