# Delivery, Retry, and Dead Letters

## Delivery model

At-least-once. Every consumer stores processed event IDs in the same transaction as its business change.

## Retry classes

- Immediate bounded retry for short network interruptions.
- Delayed retry topics for transient external failures.
- No retry for invalid schema, authorization, policy rejection, or invariant violations.

## Dead-letter record

Include original event, consumer, error category, message, stack reference, attempts, first failure, last failure, and operator disposition.

## Replay

Replay requires authorization and an audit event. The operator chooses a bounded event set, verifies current aggregate state, and uses the original event IDs so idempotent consumers remain safe.
