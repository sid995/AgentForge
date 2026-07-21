package domain

import "errors"

var (
	// ErrNotFound is returned when a resource is absent in the caller's tenant scope.
	ErrNotFound = errors.New("resource not found")
	// ErrConflict is returned when a database uniqueness constraint is violated.
	ErrConflict = errors.New("resource conflicts with existing state")
	// ErrForbidden is returned when a database policy rejects an attempted write.
	ErrForbidden = errors.New("operation is not permitted")
)
