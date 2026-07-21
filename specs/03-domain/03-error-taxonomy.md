# Error Taxonomy

## Categories

- `VALIDATION`: malformed or unsupported user input.
- `AUTHENTICATION`: missing or invalid identity.
- `AUTHORIZATION`: identity lacks permission.
- `QUOTA`: concurrency, resource, or budget limit reached.
- `CONFLICT`: state or optimistic-lock conflict.
- `TRANSIENT_DEPENDENCY`: timeout, throttling, or temporary external failure.
- `PERMANENT_DEPENDENCY`: unsupported operation or persistent rejection.
- `EXECUTION`: agent or generated code failure.
- `POLICY`: security or governance rejection.
- `INTERNAL`: invariant violation or unexpected defect.

## Retry policy

Retry only errors explicitly classified as transient. Every retry decision records attempt count, selected delay, and reason. Validation, authorization, policy rejection, and test failure are not infrastructure retries.

## AgentRun persistence errors

- `VERSION_CONFLICT`: a conditional run or attempt update lost an
  optimistic-lock race. Callers may reload and re-evaluate an idempotent command
  but must not blindly overwrite the current status.
- `IDEMPOTENCY_KEY_REUSED`: a tenant reused a create-run key with a materially
  different effective request. It is a non-retryable conflict.
- `RUN_TERMINAL`: a command requested a transition prohibited by the terminal
  outcome. The documented manual retry exception remains subject to its failure
  category and maximum-attempt guards.

## API mapping

- Validation: 400 or 422.
- Authentication: 401.
- Authorization: 403.
- Missing resource: 404.
- Conflict: 409.
- Rate or quota: 429.
- Temporary dependency: 503.
