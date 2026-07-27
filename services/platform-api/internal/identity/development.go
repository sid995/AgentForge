package identity

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
)

// DevelopmentCredential configures one explicitly local-only identity. The
// token is compared in constant time and is never logged or returned.
type DevelopmentCredential struct {
	Token    string
	Identity Identity
}

// DevelopmentResolver resolves opaque development bearer tokens. It is a
// temporary adapter; production authentication is intentionally deferred to
// the Phase 12 OIDC implementation.
type DevelopmentResolver struct {
	credentials []DevelopmentCredential
}

// NewDevelopmentResolver validates server-side development credentials.
func NewDevelopmentResolver(credentials []DevelopmentCredential) (*DevelopmentResolver, error) {
	if len(credentials) == 0 {
		return nil, fmt.Errorf("at least one development identity is required")
	}
	for index := range credentials {
		credentials[index].Token = strings.TrimSpace(credentials[index].Token)
		if credentials[index].Token == "" {
			return nil, fmt.Errorf("development identity token is required")
		}
		if err := credentials[index].Identity.Validate(); err != nil {
			return nil, fmt.Errorf("development identity is invalid: %w", err)
		}
	}
	return &DevelopmentResolver{credentials: credentials}, nil
}

// Resolve returns a trusted identity only for a configured opaque token.
func (resolver *DevelopmentResolver) Resolve(_ context.Context, credential string) (Identity, error) {
	credential = strings.TrimSpace(credential)
	for _, configured := range resolver.credentials {
		if subtle.ConstantTimeCompare([]byte(configured.Token), []byte(credential)) == 1 {
			return configured.Identity, nil
		}
	}
	return Identity{}, ErrUnauthenticated
}
