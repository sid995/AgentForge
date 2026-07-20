# LLM Agent Roles and Gates

## Suggested roles

- Planner: decomposes issue and identifies contracts.
- Implementer: produces bounded patch.
- Test author: adds adversarial and integration tests.
- Security reviewer: checks identity, isolation, injection, credentials, and supply chain.
- Reliability reviewer: checks retries, idempotency, timeouts, backpressure, and recovery.
- Documentation reviewer: verifies specs and runbooks match behavior.

## Gates

No role self-approves its own work. Generated patches pass deterministic tooling and at least one independent review pass. High-risk changes involving authentication, secrets, admission policy, production deployment, billing, or deletion require explicit human review.

## Evaluation

Track patch acceptance rate, defects found after merge, reverted changes, test quality, review findings, token cost, and time saved. Do not optimize only for generated lines of code, humanity has already suffered enough from that metric.
