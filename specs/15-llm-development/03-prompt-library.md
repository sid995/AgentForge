# Prompt Library

## Implementation prompt

```text
Implement the bounded task below using the attached AgentForge specifications.
First list requirements, invariants, affected contracts, and failure cases.
Then provide a file-level plan. Implement only after the plan.
Do not invent external APIs. Preserve backward compatibility.
Add tests for success, invalid input, duplicate execution, concurrency, and dependency failure where relevant.
Add structured telemetry and update documentation when behavior changes.
```

## Review prompt

```text
Review this patch as a senior infrastructure engineer.
Prioritize correctness, race conditions, idempotency, tenant isolation, authorization, failure recovery, resource leaks, migration safety, observability, and test gaps.
Reference exact files and lines. Separate blockers from improvements. Do not rewrite the code unless requested.
```

## Incident prompt

```text
Analyze the evidence without assuming the first hypothesis is correct.
Produce ranked hypotheses, evidence for and against each, the smallest discriminating checks, immediate safe mitigation, and long-term prevention.
Flag missing data explicitly.
```
