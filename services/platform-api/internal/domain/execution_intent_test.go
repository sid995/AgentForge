package domain

import (
	"strings"
	"testing"
)

func TestAgentRunDesiredStateValidation(t *testing.T) {
	desired := validDesiredState()
	if err := desired.Validate(1); err != nil {
		t.Fatal(err)
	}
	desired.CPULimitMillis = desired.CPUMillis - 1
	if err := desired.Validate(1); err == nil {
		t.Fatal("CPU limit below request accepted")
	}
	desired = validDesiredState()
	desired.RetryableFailureCategories = []FailureCategory{FailureInternal, FailureInternal}
	if err := desired.Validate(1); err == nil {
		t.Fatal("duplicate retry category accepted")
	}
	desired = validDesiredState()
	desired.NetworkProfile = "RestrictedEgress"
	if err := desired.Validate(1); err == nil {
		t.Fatal("restricted egress without a destination reference accepted")
	}
	desired = validDesiredState()
	desired.TaskReference = "a://task"
	if err := desired.Validate(1); err == nil {
		t.Fatal("task reference outside the CRD scheme contract accepted")
	}
	desired = validDesiredState()
	desired.ArtifactDestinationRef = strings.Repeat("a", 254)
	if err := desired.Validate(1); err == nil {
		t.Fatal("oversized object reference accepted")
	}
}

func validDesiredState() AgentRunDesiredState {
	return AgentRunDesiredState{
		RunnerImage: "registry.example.test/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Runtime:     "python-3.12", ExecutionProfile: "standard", TaskReference: "vault://tasks/example",
		TimeoutSeconds: 300, MaxAttempts: 2, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300,
		RetryableFailureCategories: []FailureCategory{FailureTransientDependency, FailureInternal},
		CPUMillis:                  1000, MemoryMiB: 2048, CPULimitMillis: 1000, MemoryLimitMiB: 2048,
		WorkspaceSizeGiB: 10, WorkspaceRetentionPolicy: "Delete", NetworkProfile: "Isolated",
		ArtifactDestinationRef: "artifact-store",
	}
}
