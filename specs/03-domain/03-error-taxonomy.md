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

## API mapping

- Validation: 400 or 422.
- Authentication: 401.
- Authorization: 403.
- Missing resource: 404.
- Conflict: 409.
- Rate or quota: 429.
- Temporary dependency: 503.
