package domain

import "errors"

var (
	// ErrValidation marks client-supplied values that fail domain validation.
	ErrValidation = errors.New("validation failed")
	// ErrNotFound is returned when a resource is absent in the caller's tenant scope.
	ErrNotFound = errors.New("resource not found")
	// ErrConflict is returned when a database uniqueness constraint is violated.
	ErrConflict = errors.New("resource conflicts with existing state")
	// ErrForbidden is returned when a database policy rejects an attempted write.
	ErrForbidden = errors.New("operation is not permitted")
	// ErrVersionConflict is returned when an optimistic update lost a race.
	ErrVersionConflict = errors.New("resource version conflicts with current state")
	// ErrInvalidTransition is returned when a command violates the aggregate state machine.
	ErrInvalidTransition = errors.New("state transition is not permitted")
	// ErrCapacityUnavailable indicates a temporary lack of execution capacity.
	ErrCapacityUnavailable = errors.New("execution capacity is unavailable")
	// ErrBudgetUnavailable indicates a temporary lack of tenant budget.
	ErrBudgetUnavailable = errors.New("tenant budget is unavailable")
	// ErrRunTerminal is returned when a command cannot change a terminal run.
	ErrRunTerminal = errors.New("run has reached a terminal state")
	// ErrRetryNotAllowed is returned when a run is not an eligible manual retry.
	ErrRetryNotAllowed = errors.New("run is not eligible for retry")
)
