package runs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestCreatePersistsTenantDerivedIdentityAndReplaysExactRequest(t *testing.T) {
	tenantID := uuid.Must(uuid.NewV7())
	projectID := uuid.Must(uuid.NewV7())
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	repository := &runRepository{}
	service := NewService(repository, projectRepository{project: domain.Project{ID: projectID, TenantID: tenantID}}, func() time.Time { return now })
	caller := identity.Identity{TenantID: tenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}
	input := validInput()

	created, replayed, err := service.Create(context.Background(), caller, projectID, input)
	if err != nil || replayed || created.CreatedBy != caller.Subject || created.TenantID != tenantID || created.Status != domain.AgentRunQueued {
		t.Fatalf("Create() run=%#v replayed=%t error=%v", created, replayed, err)
	}
	replayedRun, replayed, err := service.Create(context.Background(), caller, projectID, input)
	if err != nil || !replayed || replayedRun.ID != created.ID {
		t.Fatalf("Create() replay run=%#v replayed=%t error=%v", replayedRun, replayed, err)
	}
	input.Runtime = "node-22"
	if _, _, err := service.Create(context.Background(), caller, projectID, input); !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("Create() changed request error=%v, want ErrIdempotencyKeyReused", err)
	}
}

func TestCreateRequiresAuthorizedProject(t *testing.T) {
	tenantID := uuid.Must(uuid.NewV7())
	service := NewService(&runRepository{}, projectRepository{err: domain.ErrNotFound}, time.Now)
	caller := identity.Identity{TenantID: tenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}
	if _, _, err := service.Create(context.Background(), caller, uuid.Must(uuid.NewV7()), validInput()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create() error=%v, want ErrNotFound", err)
	}
}

func TestGetDoesNotPermitViewer(t *testing.T) {
	service := NewService(&runRepository{}, projectRepository{}, time.Now)
	caller := identity.Identity{TenantID: uuid.Must(uuid.NewV7()), Subject: "viewer@example.test", Role: "viewer"}
	if _, err := service.Get(context.Background(), caller, uuid.Must(uuid.NewV7())); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("Get() error=%v, want ErrUnauthorized", err)
	}
}

func TestCancelAndRetryDeriveTenantAndActor(t *testing.T) {
	tenantID := uuid.Must(uuid.NewV7())
	runID := uuid.Must(uuid.NewV7())
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	repository := &runRepository{commandResult: ports.RunCommandResult{Run: domain.AgentRun{ID: runID}}}
	service := NewService(repository, projectRepository{}, func() time.Time { return now })
	caller := identity.Identity{TenantID: tenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}

	if _, err := service.Cancel(context.Background(), caller, runID, "cancel-key", "requested by operator"); err != nil {
		t.Fatalf("Cancel() error=%v", err)
	}
	if repository.cancel.TenantID != tenantID || repository.cancel.RunID != runID || repository.cancel.Actor != caller.Subject || !repository.cancel.Now.Equal(now) {
		t.Fatalf("Cancel() request=%#v", repository.cancel)
	}
	if _, err := service.Retry(context.Background(), caller, runID, "retry-key"); err != nil {
		t.Fatalf("Retry() error=%v", err)
	}
	if repository.retry.TenantID != tenantID || repository.retry.RunID != runID || repository.retry.Actor != caller.Subject || !repository.retry.Now.Equal(now) {
		t.Fatalf("Retry() request=%#v", repository.retry)
	}
}

func TestCancelAndRetryRejectUnauthorizedCaller(t *testing.T) {
	service := NewService(&runRepository{}, projectRepository{}, time.Now)
	caller := identity.Identity{TenantID: uuid.Must(uuid.NewV7()), Subject: "viewer@example.test", Role: "viewer"}
	if _, err := service.Cancel(context.Background(), caller, uuid.Must(uuid.NewV7()), "cancel-key", "reason"); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("Cancel() error=%v, want ErrUnauthorized", err)
	}
	if _, err := service.Retry(context.Background(), caller, uuid.Must(uuid.NewV7()), "retry-key"); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("Retry() error=%v, want ErrUnauthorized", err)
	}
}

func validInput() CreateInput {
	return CreateInput{PromptReference: "vault://prompts/request-1", Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: "idempotency-key-1"}
}

type runRepository struct {
	created       *domain.AgentRun
	cancel        ports.CancelRunRequest
	retry         ports.RetryRunRequest
	commandResult ports.RunCommandResult
}

func (repository *runRepository) Create(_ context.Context, run domain.AgentRun, _ domain.AgentRunAttempt) error {
	if repository.created != nil {
		return domain.ErrConflict
	}
	copy := run
	repository.created = &copy
	return nil
}
func (repository *runRepository) Get(_ context.Context, _, _ uuid.UUID) (domain.AgentRun, error) {
	return domain.AgentRun{}, domain.ErrNotFound
}
func (repository *runRepository) GetByIdempotencyKey(_ context.Context, _ uuid.UUID, _ string) (domain.AgentRun, error) {
	if repository.created == nil {
		return domain.AgentRun{}, domain.ErrNotFound
	}
	return *repository.created, nil
}
func (repository *runRepository) List(context.Context, uuid.UUID, ports.AgentRunPage) ([]domain.AgentRun, error) {
	return nil, nil
}
func (repository *runRepository) Save(context.Context, domain.AgentRun, int64) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (repository *runRepository) CreateNextAttempt(context.Context, domain.AgentRun, domain.AgentRunAttempt, int64) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (repository *runRepository) GetAttempt(context.Context, uuid.UUID, uuid.UUID, int) (domain.AgentRunAttempt, error) {
	return domain.AgentRunAttempt{}, nil
}
func (repository *runRepository) ListAttempts(context.Context, uuid.UUID, uuid.UUID) ([]domain.AgentRunAttempt, error) {
	return nil, nil
}
func (repository *runRepository) SaveAttempt(context.Context, domain.AgentRunAttempt, int64) (domain.AgentRunAttempt, error) {
	return domain.AgentRunAttempt{}, nil
}
func (repository *runRepository) Cancel(_ context.Context, request ports.CancelRunRequest) (ports.RunCommandResult, error) {
	repository.cancel = request
	return repository.commandResult, nil
}
func (repository *runRepository) Retry(_ context.Context, request ports.RetryRunRequest) (ports.RunCommandResult, error) {
	repository.retry = request
	return repository.commandResult, nil
}

type projectRepository struct {
	project domain.Project
	err     error
}

func (repository projectRepository) Create(context.Context, domain.Project) error { return nil }
func (repository projectRepository) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Project, error) {
	return repository.project, repository.err
}
func (repository projectRepository) List(context.Context, uuid.UUID, ports.ProjectPage) ([]domain.Project, error) {
	return nil, nil
}
