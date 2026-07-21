# LLM-Assisted Engineering Workflow

## Principle

LLMs accelerate implementation but do not own architectural truth. The Markdown specifications, tests, schemas, and ADRs are authoritative.

## Work unit flow

1. Select one bounded issue.
2. Load only relevant specs and adjacent code.
3. Ask the LLM to restate requirements, invariants, and risks.
4. Request an implementation plan referencing exact files.
5. Generate the smallest coherent patch.
6. Run format, lint, tests, security checks, and contract validation.
7. Ask a separate review pass to find correctness, concurrency, security, and operability defects.
8. Update specs or ADRs when decisions change.
9. Commit the validated work unit before starting the next sub-phase.

## Rules

- Never paste production secrets or private source into external models without approval.
- Do not accept invented SDK methods, APIs, schemas, or Kubernetes fields.
- Require citations to repository files in plans and reviews.
- Prefer tests and executable validation over prose confidence.
- Keep changes narrow enough for human review.
- Do not carry uncommitted implementation changes across sub-phase boundaries.
