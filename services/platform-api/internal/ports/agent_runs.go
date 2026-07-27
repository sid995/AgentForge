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

// CancelRunRequest contains one durable, idempotent cancellation command.
type CancelRunRequest struct {
	TenantID  uuid.UUID
	RunID     uuid.UUID
	CommandID string
	Reason    string
	Actor     string
	Now       time.Time
}

// RetryRunRequest contains one durable, idempotent manual retry command.
type RetryRunRequest struct {
	TenantID  uuid.UUID
	RunID     uuid.UUID
	CommandID string
	Actor     string
	Now       time.Time
}

// RunCommandResult returns the stable command response and replay state.
type RunCommandResult struct {
	Run      domain.AgentRun
	Replayed bool
}

// AgentRunRepository owns tenant-scoped AgentRun and AgentRunAttempt persistence.
type AgentRunRepository interface {
	Create(context.Context, domain.AgentRun, domain.AgentRunAttempt) error
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.AgentRun, error)
	GetByIdempotencyKey(context.Context, uuid.UUID, string) (domain.AgentRun, error)
	List(context.Context, uuid.UUID, AgentRunPage) ([]domain.AgentRun, error)
	Save(context.Context, domain.AgentRun, int64) (domain.AgentRun, error)
	CreateNextAttempt(context.Context, domain.AgentRun, domain.AgentRunAttempt, int64) (domain.AgentRun, error)
	GetAttempt(context.Context, uuid.UUID, uuid.UUID, int) (domain.AgentRunAttempt, error)
	ListAttempts(context.Context, uuid.UUID, uuid.UUID) ([]domain.AgentRunAttempt, error)
	SaveAttempt(context.Context, domain.AgentRunAttempt, int64) (domain.AgentRunAttempt, error)
	Cancel(context.Context, CancelRunRequest) (RunCommandResult, error)
	Retry(context.Context, RetryRunRequest) (RunCommandResult, error)
}
