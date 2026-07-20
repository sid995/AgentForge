# AgentRun CRD Specification

## Spec fields

- tenantId, projectId, runId, attempt.
- runner image by digest.
- runtime and task reference.
- resource requests and limits.
- timeout and retry metadata.
- workspace size and storage class.
- sandbox profile.
- approved egress destinations.
- artifact destination reference.

## Status fields

- phase and reason.
- observed generation.
- Pod and Job references.
- start, completion, and heartbeat timestamps.
- conditions using Kubernetes conventions.
- summarized resource and usage values.

## Validation

Use CRD schema and admission policy to reject privileged settings, mutable image tags where prohibited, excessive resources, and invalid timeout ranges.
