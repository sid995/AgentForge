# Cloud and Terraform

## Initial provider

Implement one provider thoroughly, GCP/GKE or AWS/EKS. Maintain provider-neutral domain ports but avoid pretending every cloud difference is already abstracted.

## Terraform modules

- network.
- Kubernetes cluster and node pools.
- database.
- Redis.
- object storage.
- registry.
- DNS and certificates.
- workload identity and IAM.
- observability integrations.

## State

Remote encrypted state with locking. Separate environment state. Plans reviewed in CI. No secret values committed or exposed in normal outputs.
