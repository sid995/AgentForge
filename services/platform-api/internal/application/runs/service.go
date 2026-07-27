// Package runs implements the authenticated AgentRun command/query boundary.
package runs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// ErrIdempotencyKeyReused is returned for one key with different effective input.
var ErrIdempotencyKeyReused = errors.New("idempotency key was reused with a different request")

// Service coordinates AgentRun commands through tenant-scoped ports.
type Service struct {
	runs     ports.AgentRunRepository
	projects ports.ProjectRepository
	now      func() time.Time
}

// CreateInput is the user-controlled run request after HTTP parsing.
type CreateInput struct {
	PromptReference string
	Runtime         string
	CPUMillis       int
	MemoryMiB       int
	TimeoutSeconds  int
	MaxAttempts     int
	IdempotencyKey  string
}

// NewService constructs the run application service.
func NewService(runs ports.AgentRunRepository, projects ports.ProjectRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{runs: runs, projects: projects, now: now}
}

// Create accepts a valid run request. The boolean reports a same-request replay.
func (service *Service) Create(ctx context.Context, caller identity.Identity, projectID uuid.UUID, input CreateInput) (domain.AgentRun, bool, error) {
	if !caller.CanManageRuns() {
		return domain.AgentRun{}, false, identity.ErrUnauthorized
	}
	if _, err := service.projects.Get(ctx, caller.TenantID, projectID); err != nil {
		return domain.AgentRun{}, false, err
	}
	requestHash, err := hashRequest(projectID, input)
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	run, err := domain.NewAgentRun(domain.NewAgentRunInput{
		TenantID: caller.TenantID, ProjectID: projectID, PromptReference: input.PromptReference,
		Runtime: input.Runtime, CPUMillis: input.CPUMillis, MemoryMiB: input.MemoryMiB,
		TimeoutSeconds: input.TimeoutSeconds, MaxAttempts: input.MaxAttempts, IdempotencyKey: input.IdempotencyKey,
		RequestHash: requestHash, CreatedBy: caller.Subject,
	}, service.now())
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	attempt, err := domain.NewAgentRunAttempt(caller.TenantID, run.ID, 1, service.now())
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	if err := service.runs.Create(ctx, run, attempt); err == nil {
		return run, false, nil
	} else if !errors.Is(err, domain.ErrConflict) {
		return domain.AgentRun{}, false, err
	}

	existing, err := service.runs.GetByIdempotencyKey(ctx, caller.TenantID, input.IdempotencyKey)
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	if existing.RequestHash != requestHash {
		return domain.AgentRun{}, false, ErrIdempotencyKeyReused
	}
	return existing, true, nil
}

// Get returns a run only inside the caller's tenant scope.
func (service *Service) Get(ctx context.Context, caller identity.Identity, runID uuid.UUID) (domain.AgentRun, error) {
	if !caller.CanManageRuns() {
		return domain.AgentRun{}, identity.ErrUnauthorized
	}
	return service.runs.Get(ctx, caller.TenantID, runID)
}

// List returns a project-scoped run page only within the caller's tenant.
func (service *Service) List(ctx context.Context, caller identity.Identity, projectID uuid.UUID, page ports.AgentRunPage) ([]domain.AgentRun, error) {
	if !caller.CanManageRuns() {
		return nil, identity.ErrUnauthorized
	}
	if _, err := service.projects.Get(ctx, caller.TenantID, projectID); err != nil {
		return nil, err
	}
	page.ProjectID = &projectID
	return service.runs.List(ctx, caller.TenantID, page)
}

// Cancel records a durable cancellation request. It does not contact Kubernetes.
func (service *Service) Cancel(ctx context.Context, caller identity.Identity, runID uuid.UUID, commandID, reason string) (ports.RunCommandResult, error) {
	if !caller.CanManageRuns() {
		return ports.RunCommandResult{}, identity.ErrUnauthorized
	}
	return service.runs.Cancel(ctx, ports.CancelRunRequest{TenantID: caller.TenantID, RunID: runID, CommandID: commandID, Reason: reason, Actor: caller.Subject, Now: service.now()})
}

// Retry creates the next attempt only for an explicitly retryable terminal run.
func (service *Service) Retry(ctx context.Context, caller identity.Identity, runID uuid.UUID, commandID string) (ports.RunCommandResult, error) {
	if !caller.CanManageRuns() {
		return ports.RunCommandResult{}, identity.ErrUnauthorized
	}
	return service.runs.Retry(ctx, ports.RetryRunRequest{TenantID: caller.TenantID, RunID: runID, CommandID: commandID, Actor: caller.Subject, Now: service.now()})
}

func hashRequest(projectID uuid.UUID, input CreateInput) (string, error) {
	payload, err := json.Marshal(struct {
		ProjectID       uuid.UUID `json:"projectId"`
		PromptReference string    `json:"promptRef"`
		Runtime         string    `json:"runtime"`
		CPUMillis       int       `json:"cpuMillis"`
		MemoryMiB       int       `json:"memoryMiB"`
		TimeoutSeconds  int       `json:"timeoutSeconds"`
		MaxAttempts     int       `json:"maxAttempts"`
	}{projectID, input.PromptReference, input.Runtime, input.CPUMillis, input.MemoryMiB, input.TimeoutSeconds, input.MaxAttempts})
	if err != nil {
		return "", fmt.Errorf("marshal effective run request: %w", err)
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
