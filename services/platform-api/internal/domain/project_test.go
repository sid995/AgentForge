package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewProjectValidatesTenantAndFields(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	project, err := NewProject(uuid.Must(uuid.NewV7()), " Platform API ", " https://example.test/repo.git ", now)
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if project.ID.Version() != 7 || project.Name != "Platform API" || project.RepositoryURL != "https://example.test/repo.git" || !project.CreatedAt.Equal(now) || project.Version != 1 {
		t.Fatalf("NewProject() = %#v", project)
	}

	if _, err := NewProject(uuid.Nil, "project", "", now); err == nil {
		t.Fatal("NewProject() accepted a missing tenant ID")
	}
	if _, err := NewProject(uuid.Must(uuid.NewV7()), "", "", now); err == nil {
		t.Fatal("NewProject() accepted an empty name")
	}
}

func TestNewTenantValidatesSlug(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	tenant, err := NewTenant(" Platform-Team ", "Platform Team", now)
	if err != nil {
		t.Fatalf("NewTenant() error = %v", err)
	}
	if tenant.ID.Version() != 7 || tenant.Slug != "platform-team" {
		t.Fatalf("NewTenant() = %#v", tenant)
	}
	if _, err := NewTenant("no", "Too short", now); err == nil {
		t.Fatal("NewTenant() accepted a short slug")
	}
}
