package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

// TenantRepository persists platform-provisioned tenant ownership boundaries.
type TenantRepository interface {
	Create(context.Context, domain.Tenant) error
}

// ProjectRepository only exposes tenant-scoped project operations.
type ProjectRepository interface {
	Create(context.Context, domain.Project) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Project, error)
	List(context.Context, uuid.UUID, ProjectPage) ([]domain.Project, error)
}

// ProjectPage is a keyset pagination request ordered by creation time then ID.
type ProjectPage struct {
	AfterCreatedAt *time.Time
	AfterID        *uuid.UUID
	Limit          int
}
