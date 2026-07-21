package projects

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestCreateUsesTenantScopedRepository(t *testing.T) {
	repository := &fakeRepository{}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	service := NewService(repository, func() time.Time { return now })
	tenantID := uuid.Must(uuid.NewV7())

	project, err := service.Create(context.Background(), tenantID, CreateInput{Name: "Platform", RepositoryURL: "https://example.test/repo.git"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.created.TenantID != tenantID || project.ID != repository.created.ID {
		t.Fatalf("Create() project = %#v, repository project = %#v", project, repository.created)
	}
}

type fakeRepository struct{ created domain.Project }

func (repository *fakeRepository) Create(_ context.Context, project domain.Project) error {
	repository.created = project
	return nil
}

func (repository *fakeRepository) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Project, error) {
	return domain.Project{}, domain.ErrNotFound
}

func (repository *fakeRepository) List(context.Context, uuid.UUID, ports.ProjectPage) ([]domain.Project, error) {
	return nil, nil
}
