package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AgentRunStatus is the durable lifecycle status of an AgentRun aggregate.
type AgentRunStatus string

const (
	AgentRunQueued             AgentRunStatus = "QUEUED"
	AgentRunScheduling         AgentRunStatus = "SCHEDULING"
	AgentRunCapacityWait       AgentRunStatus = "CAPACITY_WAIT"
	AgentRunProvisioning       AgentRunStatus = "PROVISIONING"
	AgentRunRunning            AgentRunStatus = "RUNNING"
	AgentRunTesting            AgentRunStatus = "TESTING"
	AgentRunBuilding           AgentRunStatus = "BUILDING"
	AgentRunScanning           AgentRunStatus = "SCANNING"
	AgentRunReadyToDeploy      AgentRunStatus = "READY_TO_DEPLOY"
	AgentRunDeploying          AgentRunStatus = "DEPLOYING"
	AgentRunCancelling         AgentRunStatus = "CANCELLING"
	AgentRunSucceeded          AgentRunStatus = "SUCCEEDED"
	AgentRunCancelled          AgentRunStatus = "CANCELLED"
	AgentRunProvisioningFailed AgentRunStatus = "PROVISIONING_FAILED"
	AgentRunExecutionFailed    AgentRunStatus = "EXECUTION_FAILED"
	AgentRunBuildFailed        AgentRunStatus = "BUILD_FAILED"
	AgentRunPolicyRejected     AgentRunStatus = "POLICY_REJECTED"
	AgentRunDeploymentFailed   AgentRunStatus = "DEPLOYMENT_FAILED"
	AgentRunTimedOut           AgentRunStatus = "TIMED_OUT"
)

// FailureCategory is the normalized, non-sensitive cause class for a failed run or attempt.
type FailureCategory string

const (
	FailureValidation          FailureCategory = "VALIDATION"
	FailureAuthentication      FailureCategory = "AUTHENTICATION"
	FailureAuthorization       FailureCategory = "AUTHORIZATION"
	FailureQuota               FailureCategory = "QUOTA"
	FailureConflict            FailureCategory = "CONFLICT"
	FailureTransientDependency FailureCategory = "TRANSIENT_DEPENDENCY"
	FailurePermanentDependency FailureCategory = "PERMANENT_DEPENDENCY"
	FailureExecution           FailureCategory = "EXECUTION"
	FailurePolicy              FailureCategory = "POLICY"
	FailureInternal            FailureCategory = "INTERNAL"
)

// IsValid reports whether a failure category belongs to the stable taxonomy.
func (category FailureCategory) IsValid() bool {
	switch category {
	case FailureValidation, FailureAuthentication, FailureAuthorization, FailureQuota, FailureConflict, FailureTransientDependency, FailurePermanentDependency, FailureExecution, FailurePolicy, FailureInternal:
		return true
	default:
		return false
	}
}

// AgentRunAttemptStatus is the durable lifecycle status of one execution attempt.
type AgentRunAttemptStatus string

const (
	AttemptPending   AgentRunAttemptStatus = "PENDING"
	AttemptStarting  AgentRunAttemptStatus = "STARTING"
	AttemptActive    AgentRunAttemptStatus = "ACTIVE"
	AttemptCompleted AgentRunAttemptStatus = "COMPLETED"
	AttemptFailed    AgentRunAttemptStatus = "FAILED"
	AttemptCancelled AgentRunAttemptStatus = "CANCELLED"
	AttemptTimedOut  AgentRunAttemptStatus = "TIMED_OUT"
)

// AgentRunDesiredState is the complete immutable Scheduler intent handed to
// Kubernetes after a cluster assignment commits.
type AgentRunDesiredState struct {
	RunnerImage                string            `json:"runnerImage"`
	Runtime                    string            `json:"runtime"`
	ExecutionProfile           string            `json:"executionProfile"`
	TaskReference              string            `json:"taskRef"`
	TimeoutSeconds             int               `json:"timeoutSeconds"`
	MaxAttempts                int               `json:"maxAttempts"`
	InitialBackoffSeconds      int               `json:"initialBackoffSeconds"`
	MaxBackoffSeconds          int               `json:"maxBackoffSeconds"`
	RetryableFailureCategories []FailureCategory `json:"retryableFailureCategories,omitempty"`
	CPUMillis                  int               `json:"cpuMillis"`
	MemoryMiB                  int               `json:"memoryMiB"`
	CPULimitMillis             int               `json:"cpuLimitMillis"`
	MemoryLimitMiB             int               `json:"memoryLimitMiB"`
	WorkspaceSizeGiB           int               `json:"workspaceSizeGiB"`
	StorageClassName           string            `json:"storageClassName,omitempty"`
	WorkspaceRetentionPolicy   string            `json:"workspaceRetentionPolicy"`
	NetworkProfile             string            `json:"networkProfile"`
	AllowedDestinationsRef     string            `json:"allowedDestinationsRef,omitempty"`
	ArtifactDestinationRef     string            `json:"artifactDestinationRef"`
	ConfigurationRefs          []string          `json:"configurationRefs,omitempty"`
	SecretRefs                 []string          `json:"secretRefs,omitempty"`
	DeployOnSuccess            bool              `json:"deployOnSuccess"`
}

// AgentRun is the tenant-owned durable aggregate for one accepted task request.
type AgentRun struct {
	ID                      uuid.UUID
	TenantID                uuid.UUID
	ProjectID               uuid.UUID
	PromptReference         string
	Runtime                 string
	CPUMillis               int
	MemoryMiB               int
	TimeoutSeconds          int
	MaxAttempts             int
	AttemptCount            int
	Status                  AgentRunStatus
	FailureCategory         FailureCategory
	IdempotencyKey          string
	RequestHash             string
	Version                 int64
	CreatedBy               string
	CancellationReason      string
	CancellationRequestedBy string
	CancellationRequestedAt *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
	CompletedAt             *time.Time
}

// NewAgentRunInput is the validated input boundary for a durable run request.
type NewAgentRunInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	PromptReference string
	Runtime         string
	CPUMillis       int
	MemoryMiB       int
	TimeoutSeconds  int
	MaxAttempts     int
	IdempotencyKey  string
	RequestHash     string
	CreatedBy       string
}

// NewAgentRun validates request metadata and constructs a queued aggregate.
func NewAgentRun(input NewAgentRunInput, now time.Time) (AgentRun, error) {
	input.PromptReference = strings.TrimSpace(input.PromptReference)
	input.Runtime = strings.TrimSpace(input.Runtime)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.RequestHash = strings.TrimSpace(input.RequestHash)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.TenantID == uuid.Nil || input.ProjectID == uuid.Nil {
		return AgentRun{}, fmt.Errorf("run tenant and project IDs are required")
	}
	if len(input.PromptReference) == 0 || len(input.PromptReference) > 2048 {
		return AgentRun{}, fmt.Errorf("prompt reference must contain 1 to 2048 characters")
	}
	if len(input.Runtime) == 0 || len(input.Runtime) > 120 {
		return AgentRun{}, fmt.Errorf("runtime must contain 1 to 120 characters")
	}
	if input.CPUMillis < 1 || input.CPUMillis > 128000 || input.MemoryMiB < 1 || input.MemoryMiB > 524288 {
		return AgentRun{}, fmt.Errorf("resource request is outside supported bounds")
	}
	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > 86400 {
		return AgentRun{}, fmt.Errorf("timeout must be between 1 and 86400 seconds")
	}
	if input.MaxAttempts < 1 || input.MaxAttempts > 10 {
		return AgentRun{}, fmt.Errorf("maximum attempts must be between 1 and 10")
	}
	if len(input.IdempotencyKey) == 0 || len(input.IdempotencyKey) > 255 || len(input.RequestHash) == 0 || len(input.RequestHash) > 128 || len(input.CreatedBy) == 0 || len(input.CreatedBy) > 255 {
		return AgentRun{}, fmt.Errorf("idempotency key, request hash, and actor are required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AgentRun{}, fmt.Errorf("generate run ID: %w", err)
	}
	now = now.UTC()
	return AgentRun{ID: id, TenantID: input.TenantID, ProjectID: input.ProjectID, PromptReference: input.PromptReference, Runtime: input.Runtime, CPUMillis: input.CPUMillis, MemoryMiB: input.MemoryMiB, TimeoutSeconds: input.TimeoutSeconds, MaxAttempts: input.MaxAttempts, AttemptCount: 1, Status: AgentRunQueued, IdempotencyKey: input.IdempotencyKey, RequestHash: input.RequestHash, Version: 1, CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now}, nil
}

// IsTerminal reports whether a status has reached a stable outcome.
func (status AgentRunStatus) IsTerminal() bool {
	switch status {
	case AgentRunSucceeded, AgentRunCancelled, AgentRunProvisioningFailed, AgentRunExecutionFailed, AgentRunBuildFailed, AgentRunPolicyRejected, AgentRunDeploymentFailed, AgentRunTimedOut:
		return true
	default:
		return false
	}
}

// CanTransition applies the authoritative transition graph without mutating the aggregate.
func (run AgentRun) CanTransition(target AgentRunStatus) bool {
	if run.Status == target {
		return true
	}
	if target == AgentRunCancelling {
		return !run.Status.IsTerminal() && run.Status != AgentRunCancelling
	}
	switch run.Status {
	case AgentRunQueued:
		return target == AgentRunScheduling
	case AgentRunScheduling:
		return target == AgentRunCapacityWait || target == AgentRunProvisioning || target == AgentRunPolicyRejected || target == AgentRunTimedOut
	case AgentRunCapacityWait:
		return target == AgentRunScheduling || target == AgentRunTimedOut
	case AgentRunProvisioning:
		return target == AgentRunRunning || target == AgentRunProvisioningFailed || target == AgentRunTimedOut
	case AgentRunRunning:
		return target == AgentRunTesting || target == AgentRunExecutionFailed || target == AgentRunTimedOut
	case AgentRunTesting:
		return target == AgentRunBuilding || target == AgentRunExecutionFailed || target == AgentRunTimedOut
	case AgentRunBuilding:
		return target == AgentRunScanning || target == AgentRunBuildFailed || target == AgentRunTimedOut
	case AgentRunScanning:
		return target == AgentRunReadyToDeploy || target == AgentRunPolicyRejected || target == AgentRunTimedOut
	case AgentRunReadyToDeploy:
		return target == AgentRunDeploying
	case AgentRunDeploying:
		return target == AgentRunSucceeded || target == AgentRunDeploymentFailed || target == AgentRunTimedOut
	case AgentRunCancelling:
		return target == AgentRunCancelled
	case AgentRunProvisioningFailed, AgentRunExecutionFailed, AgentRunBuildFailed, AgentRunDeploymentFailed, AgentRunTimedOut:
		return target == AgentRunQueued && run.CanRetry()
	default:
		return false
	}
}

// Transition validates and applies a lifecycle state change.
func (run *AgentRun) Transition(target AgentRunStatus, failure FailureCategory, now time.Time) error {
	if !run.CanTransition(target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, run.Status, target)
	}
	if target == run.Status {
		return nil
	}
	if target.IsTerminal() && target != AgentRunSucceeded && target != AgentRunCancelled {
		if failure == "" {
			return fmt.Errorf("failure category is required for %s", target)
		}
		if !failure.IsValid() {
			return fmt.Errorf("failure category %q is not valid", failure)
		}
	}
	if target == AgentRunQueued && run.Status.IsTerminal() {
		run.AttemptCount++
		run.FailureCategory = ""
		run.CompletedAt = nil
	} else if target.IsTerminal() {
		now = now.UTC()
		run.CompletedAt = &now
		run.FailureCategory = failure
	}
	run.Status = target
	run.UpdatedAt = now.UTC()
	return nil
}

// CanRetry reports whether the documented manual-retry exception applies.
func (run AgentRun) CanRetry() bool {
	if !run.Status.IsTerminal() || run.AttemptCount >= run.MaxAttempts {
		return false
	}
	return run.FailureCategory == FailureTransientDependency
}

// AgentRunAttempt is one durable, monotonic execution history entry.
type AgentRunAttempt struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	RunID                 uuid.UUID
	AttemptNumber         int
	Status                AgentRunAttemptStatus
	FailureCategory       FailureCategory
	Version               int64
	SelectedCluster       string
	ExecutionProfile      string
	CapacityReservationID uuid.UUID
	BudgetReservationID   uuid.UUID
	WorkloadReference     string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	StartedAt             *time.Time
	CompletedAt           *time.Time
}

// NewAgentRunAttempt creates the pending attempt for a validated run number.
func NewAgentRunAttempt(tenantID, runID uuid.UUID, number int, now time.Time) (AgentRunAttempt, error) {
	if tenantID == uuid.Nil || runID == uuid.Nil || number < 1 {
		return AgentRunAttempt{}, fmt.Errorf("attempt tenant, run, and number are required")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return AgentRunAttempt{}, fmt.Errorf("generate attempt ID: %w", err)
	}
	now = now.UTC()
	return AgentRunAttempt{ID: id, TenantID: tenantID, RunID: runID, AttemptNumber: number, Status: AttemptPending, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

// CanTransition applies the attempt lifecycle graph.
func (attempt AgentRunAttempt) CanTransition(target AgentRunAttemptStatus) bool {
	if attempt.Status == target {
		return true
	}
	switch attempt.Status {
	case AttemptPending:
		return target == AttemptStarting || target == AttemptCancelled || target == AttemptTimedOut
	case AttemptStarting:
		return target == AttemptActive || target == AttemptFailed || target == AttemptCancelled || target == AttemptTimedOut
	case AttemptActive:
		return target == AttemptCompleted || target == AttemptFailed || target == AttemptCancelled || target == AttemptTimedOut
	default:
		return false
	}
}

// Transition validates and applies an attempt lifecycle state change.
func (attempt *AgentRunAttempt) Transition(target AgentRunAttemptStatus, failure FailureCategory, now time.Time) error {
	if !attempt.CanTransition(target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, attempt.Status, target)
	}
	if target == attempt.Status {
		return nil
	}
	now = now.UTC()
	if target == AttemptActive {
		attempt.StartedAt = &now
	}
	if target == AttemptFailed || target == AttemptTimedOut {
		if failure == "" {
			return fmt.Errorf("failure category is required for %s", target)
		}
		if !failure.IsValid() {
			return fmt.Errorf("failure category %q is not valid", failure)
		}
		attempt.FailureCategory = failure
	}
	if target == AttemptCompleted || target == AttemptFailed || target == AttemptCancelled || target == AttemptTimedOut {
		attempt.CompletedAt = &now
	}
	attempt.Status = target
	attempt.UpdatedAt = now
	return nil
}
