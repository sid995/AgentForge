// Package projects implements tenant-scoped project use cases.
package projects

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// Service coordinates validated project operations through a domain-specific port.
type Service struct {
	repository ports.ProjectRepository
	now        func() time.Time
}

// CreateInput contains the validated project creation fields.
type CreateInput struct {
	Name          string
	RepositoryURL string
}

// NewService constructs project use cases with an injectable clock.
func NewService(repository ports.ProjectRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

// Create validates and persists a project inside its tenant boundary.
func (service *Service) Create(ctx context.Context, tenantID uuid.UUID, input CreateInput) (domain.Project, error) {
	project, err := domain.NewProject(tenantID, input.Name, input.RepositoryURL, service.now())
	if err != nil {
		return domain.Project{}, err
	}
	if err := service.repository.Create(ctx, project); err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

// Get fetches a project only within the supplied tenant boundary.
func (service *Service) Get(ctx context.Context, tenantID, projectID uuid.UUID) (domain.Project, error) {
	return service.repository.Get(ctx, tenantID, projectID)
}

// List fetches a tenant-scoped project page.
func (service *Service) List(ctx context.Context, tenantID uuid.UUID, page ports.ProjectPage) ([]domain.Project, error) {
	return service.repository.List(ctx, tenantID, page)
}
