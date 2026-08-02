package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDevelopmentResolverReturnsOnlyConfiguredIdentity(t *testing.T) {
	tenantID := uuid.Must(uuid.NewV7())
	resolver, err := NewDevelopmentResolver([]DevelopmentCredential{{Token: "development-token", Identity: Identity{TenantID: tenantID, Subject: "developer@example.test", Role: RoleDeveloper}}})
	if err != nil {
		t.Fatalf("NewDevelopmentResolver() error = %v", err)
	}

	identity, err := resolver.Resolve(context.Background(), "development-token")
	if err != nil || identity.TenantID != tenantID || identity.Subject != "developer@example.test" {
		t.Fatalf("Resolve() identity=%#v error=%v", identity, err)
	}
	if _, err := resolver.Resolve(context.Background(), "client-supplied-tenant"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Resolve() invalid token error=%v, want ErrUnauthenticated", err)
	}
}

func TestDevelopmentResolverRejectsIncompleteCredential(t *testing.T) {
	_, err := NewDevelopmentResolver([]DevelopmentCredential{{Token: "token", Identity: Identity{Subject: "developer@example.test", Role: RoleDeveloper}}})
	if err == nil {
		t.Fatal("NewDevelopmentResolver() error = nil, want validation error")
	}
}

func TestOnlyProjectAdministratorsCanDeleteArtifacts(t *testing.T) {
	if (Identity{Role: RoleDeveloper}).CanManageArtifacts() {
		t.Fatal("developer can delete artifacts")
	}
	if !(Identity{Role: RoleProjectAdministrator}).CanManageArtifacts() {
		t.Fatal("project administrator cannot delete artifacts")
	}
}
