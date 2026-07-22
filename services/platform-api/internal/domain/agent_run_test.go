package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewAgentRunCreatesQueuedFirstAttemptMetadata(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	run, err := NewAgentRun(validRunInput(), now)
	if err != nil {
		t.Fatalf("NewAgentRun() error = %v", err)
	}
	if run.ID.Version() != 7 || run.Status != AgentRunQueued || run.AttemptCount != 1 || run.Version != 1 || run.PromptReference != "vault://prompts/run-1" || !run.CreatedAt.Equal(now) {
		t.Fatalf("NewAgentRun() = %#v", run)
	}
	input := validRunInput()
	input.PromptReference = ""
	if _, err := NewAgentRun(input, now); err == nil {
		t.Fatal("NewAgentRun() accepted an empty prompt reference")
	}
	input = validRunInput()
	input.MaxAttempts = 11
	if _, err := NewAgentRun(input, now); err == nil {
		t.Fatal("NewAgentRun() accepted excessive attempts")
	}
}

func TestAgentRunAllowsOnlyDocumentedTransitions(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	for _, transition := range []struct {
		from AgentRunStatus
		to   AgentRunStatus
	}{
		{AgentRunQueued, AgentRunScheduling}, {AgentRunScheduling, AgentRunCapacityWait}, {AgentRunScheduling, AgentRunProvisioning}, {AgentRunScheduling, AgentRunPolicyRejected}, {AgentRunScheduling, AgentRunTimedOut}, {AgentRunCapacityWait, AgentRunScheduling}, {AgentRunCapacityWait, AgentRunTimedOut}, {AgentRunProvisioning, AgentRunRunning}, {AgentRunProvisioning, AgentRunProvisioningFailed}, {AgentRunProvisioning, AgentRunTimedOut}, {AgentRunRunning, AgentRunTesting}, {AgentRunRunning, AgentRunExecutionFailed}, {AgentRunRunning, AgentRunTimedOut}, {AgentRunTesting, AgentRunBuilding}, {AgentRunTesting, AgentRunExecutionFailed}, {AgentRunTesting, AgentRunTimedOut}, {AgentRunBuilding, AgentRunScanning}, {AgentRunBuilding, AgentRunBuildFailed}, {AgentRunBuilding, AgentRunTimedOut}, {AgentRunScanning, AgentRunReadyToDeploy}, {AgentRunScanning, AgentRunPolicyRejected}, {AgentRunScanning, AgentRunTimedOut}, {AgentRunReadyToDeploy, AgentRunDeploying}, {AgentRunDeploying, AgentRunSucceeded}, {AgentRunDeploying, AgentRunDeploymentFailed}, {AgentRunDeploying, AgentRunTimedOut}, {AgentRunCancelling, AgentRunCancelled},
	} {
		run := AgentRun{Status: transition.from, MaxAttempts: 2, AttemptCount: 1}
		failure := FailureCategory("")
		if transition.to.IsTerminal() && transition.to != AgentRunSucceeded && transition.to != AgentRunCancelled {
			failure = FailureTransientDependency
		}
		if err := run.Transition(transition.to, failure, now); err != nil {
			t.Errorf("Transition(%s -> %s) error = %v", transition.from, transition.to, err)
		}
	}
	for _, status := range []AgentRunStatus{AgentRunQueued, AgentRunScheduling, AgentRunCapacityWait, AgentRunProvisioning, AgentRunRunning, AgentRunTesting, AgentRunBuilding, AgentRunScanning, AgentRunReadyToDeploy, AgentRunDeploying} {
		run := AgentRun{Status: status, MaxAttempts: 2, AttemptCount: 1}
		if err := run.Transition(AgentRunCancelling, "", now); err != nil {
			t.Errorf("Transition(%s -> CANCELLING) error = %v", status, err)
		}
	}
	for _, status := range []AgentRunStatus{AgentRunProvisioningFailed, AgentRunExecutionFailed, AgentRunBuildFailed, AgentRunDeploymentFailed, AgentRunTimedOut} {
		run := AgentRun{Status: status, FailureCategory: FailureTransientDependency, MaxAttempts: 2, AttemptCount: 1}
		if err := run.Transition(AgentRunQueued, "", now); err != nil {
			t.Errorf("Transition(%s -> QUEUED) error = %v", status, err)
		}
	}
	run := AgentRun{Status: AgentRunRunning, MaxAttempts: 2, AttemptCount: 1}
	if err := run.Transition(AgentRunSucceeded, "", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("illegal transition error = %v, want ErrInvalidTransition", err)
	}
}

func TestAgentRunRetryPreservesTerminalHistoryAndLimitsAttempts(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	run := AgentRun{Status: AgentRunExecutionFailed, FailureCategory: FailureTransientDependency, AttemptCount: 1, MaxAttempts: 2, CompletedAt: &now}
	if !run.CanRetry() {
		t.Fatal("CanRetry() = false for transient terminal failure")
	}
	if err := run.Transition(AgentRunQueued, "", now.Add(time.Minute)); err != nil {
		t.Fatalf("retry Transition() error = %v", err)
	}
	if run.AttemptCount != 2 || run.FailureCategory != "" || run.CompletedAt != nil {
		t.Fatalf("retried run = %#v", run)
	}
	run = AgentRun{Status: AgentRunExecutionFailed, FailureCategory: FailureExecution, AttemptCount: 1, MaxAttempts: 2}
	if run.CanRetry() || run.CanTransition(AgentRunQueued) {
		t.Fatal("execution failure was retryable")
	}
	run = AgentRun{Status: AgentRunTimedOut, FailureCategory: FailureTransientDependency, AttemptCount: 2, MaxAttempts: 2}
	if run.CanRetry() {
		t.Fatal("run above max attempts was retryable")
	}
}

func TestAgentRunAttemptTransitionsAreExplicit(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	for _, transition := range []struct {
		from AgentRunAttemptStatus
		to   AgentRunAttemptStatus
	}{
		{AttemptPending, AttemptStarting}, {AttemptPending, AttemptCancelled}, {AttemptPending, AttemptTimedOut}, {AttemptStarting, AttemptActive}, {AttemptStarting, AttemptFailed}, {AttemptStarting, AttemptCancelled}, {AttemptStarting, AttemptTimedOut}, {AttemptActive, AttemptCompleted}, {AttemptActive, AttemptFailed}, {AttemptActive, AttemptCancelled}, {AttemptActive, AttemptTimedOut},
	} {
		attempt := AgentRunAttempt{Status: transition.from}
		failure := FailureCategory("")
		if transition.to == AttemptFailed || transition.to == AttemptTimedOut {
			failure = FailureTransientDependency
		}
		if err := attempt.Transition(transition.to, failure, now); err != nil {
			t.Errorf("Attempt Transition(%s -> %s) error = %v", transition.from, transition.to, err)
		}
	}
	attempt := AgentRunAttempt{Status: AttemptActive}
	if err := attempt.Transition(AttemptCompleted, "", now); err != nil {
		t.Fatalf("active -> completed: %v", err)
	}
	if err := attempt.Transition(AttemptActive, "", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal attempt transition error = %v, want ErrInvalidTransition", err)
	}
	attempt = AgentRunAttempt{Status: AttemptActive}
	if err := attempt.Transition(AttemptFailed, "", now); err == nil {
		t.Fatal("failed attempt did not require a failure category")
	}
	attempt = AgentRunAttempt{Status: AttemptActive}
	if err := attempt.Transition(AttemptFailed, "unknown", now); err == nil {
		t.Fatal("failed attempt accepted an unknown failure category")
	}
}

func validRunInput() NewAgentRunInput {
	return NewAgentRunInput{TenantID: uuid.Must(uuid.NewV7()), ProjectID: uuid.Must(uuid.NewV7()), PromptReference: "vault://prompts/run-1", Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 2, IdempotencyKey: "run-1", RequestHash: "sha256:request-1", CreatedBy: "developer@example.test"}
}
