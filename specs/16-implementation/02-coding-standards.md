# Coding Standards

## Go

- Explicit dependency injection and narrow interfaces.
- Context passed to blocking or remote operations.
- Errors wrapped with stable categories while preserving cause.
- No package-global mutable business state.
- Graceful shutdown and bounded goroutines.
- Table-driven tests and race testing for concurrent code.
- SQL written or generated transparently; critical queries reviewed.

## Contracts

- JSON fields use stable documented names.
- Time is UTC and injected through a clock port in domain tests.
- IDs are generated through an injectable provider.
- Events and APIs use separate DTOs from domain entities.

## Logging

Structured fields, no secret values, no unbounded payloads, and no logging every reconcile loop without meaningful change.
