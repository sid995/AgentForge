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

## Phase 6.1 bootstrap contract

The namespaced group/version is `execution.agentforge.dev/v1alpha1`. Phase 6.1
contains an intentionally empty required `spec` skeleton and conventional
condition status solely to prove generation, scheme registration, status
subresource wiring, and envtest serving. It is not the approved executable
workload contract and cannot cause workload creation. Phase 6.2 replaces the
empty skeleton with all fields and validations listed above.
