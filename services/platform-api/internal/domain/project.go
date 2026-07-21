package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Project is a tenant-owned mutable aggregate.
type Project struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Name          string
	RepositoryURL string
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewProject validates project attributes and assigns a time-sortable UUIDv7.
func NewProject(tenantID uuid.UUID, name, repositoryURL string, now time.Time) (Project, error) {
	name = strings.TrimSpace(name)
	repositoryURL = strings.TrimSpace(repositoryURL)
	if tenantID == uuid.Nil {
		return Project{}, fmt.Errorf("project tenant ID is required")
	}
	if name == "" || len(name) > 120 {
		return Project{}, fmt.Errorf("project name must contain 1 to 120 characters")
	}
	if len(repositoryURL) > 2048 {
		return Project{}, fmt.Errorf("project repository URL must contain at most 2048 characters")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Project{}, fmt.Errorf("generate project ID: %w", err)
	}
	now = now.UTC()
	return Project{ID: id, TenantID: tenantID, Name: name, RepositoryURL: repositoryURL, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}
