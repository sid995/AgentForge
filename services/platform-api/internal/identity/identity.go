// Package identity defines the temporary boundary used to obtain a trusted
// caller identity. HTTP adapters must never derive tenancy from request input.
package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	// ErrUnauthenticated means a request did not present a valid credential.
	ErrUnauthenticated = errors.New("identity is not authenticated")
	// ErrUnauthorized means an authenticated identity lacks the required role.
	ErrUnauthorized = errors.New("identity is not authorized")
)

// Role is the temporary coarse-grained permission granted by development
// identity. Phase 12 replaces this with OIDC membership and resource RBAC.
type Role string

const (
	RoleDeveloper            Role = "developer"
	RoleProjectAdministrator Role = "project-administrator"
)

// Identity is server-derived caller context used by application services.
type Identity struct {
	TenantID uuid.UUID
	Subject  string
	Role     Role
}

// CanManageRuns reports whether the temporary role may create or read runs.
func (identity Identity) CanManageRuns() bool {
	return identity.Role == RoleDeveloper || identity.Role == RoleProjectAdministrator
}

// Validate ensures an identity cannot be constructed with an incomplete scope.
func (identity Identity) Validate() error {
	if identity.TenantID == uuid.Nil || strings.TrimSpace(identity.Subject) == "" || !identity.CanManageRuns() {
		return ErrUnauthenticated
	}
	return nil
}

// Resolver obtains an identity from an opaque authorization credential.
type Resolver interface {
	Resolve(context.Context, string) (Identity, error)
}
