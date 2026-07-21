package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

// AgentRunPage is a keyset pagination request ordered by creation time then ID.
type AgentRunPage struct {
	ProjectID      *uuid.UUID
	AfterCreatedAt *time.Time
	AfterID        *uuid.UUID
	Limit          int
}

// AgentRunRepository owns tenant-scoped AgentRun and AgentRunAttempt persistence.
type AgentRunRepository interface {
	Create(context.Context, domain.AgentRun, domain.AgentRunAttempt) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.AgentRun, error)
	List(context.Context, uuid.UUID, AgentRunPage) ([]domain.AgentRun, error)
	Save(context.Context, domain.AgentRun, int64) (domain.AgentRun, error)
	CreateNextAttempt(context.Context, domain.AgentRun, domain.AgentRunAttempt, int64) (domain.AgentRun, error)
	GetAttempt(context.Context, uuid.UUID, uuid.UUID, int) (domain.AgentRunAttempt, error)
	ListAttempts(context.Context, uuid.UUID, uuid.UUID) ([]domain.AgentRunAttempt, error)
	SaveAttempt(context.Context, domain.AgentRunAttempt, int64) (domain.AgentRunAttempt, error)
}
