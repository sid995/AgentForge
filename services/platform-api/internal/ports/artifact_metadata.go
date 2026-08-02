package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

// ArtifactRepository persists immutable, tenant-scoped artifact metadata.
type ArtifactRepository interface {
	Create(context.Context, domain.Artifact) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Artifact, error)
	ListByRun(context.Context, uuid.UUID, uuid.UUID) ([]domain.Artifact, error)
	RecordDeletion(context.Context, domain.ArtifactDeletion) error
}
