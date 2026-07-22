package domain

import (
	"testing"
	"time"
)

func TestEligibilityPolicyBoundariesAndExplanations(t *testing.T) {
	now := time.Date(2026, 7, 22, 13, 0, 0, 0, time.UTC)
	policy := SchedulingPolicy{MaxConcurrentRuns: 2, MaxUserConcurrentRuns: 1, MaxQueuedRuns: 3, MaxCPUMillis: 2000, MaxMemoryMiB: 4096, DailyBudgetMinorUnits: 1000, AllowedRuntimes: []string{"python-3.12"}, AllowedProfiles: []string{"standard"}, DeferralDuration: time.Minute}
	valid := EligibilityInput{TenantStatus: SchedulerResourceStatusActive, ProjectStatus: SchedulerResourceStatusActive, Request: SchedulingRequest{Runtime: "python-3.12", ExecutionProfile: "standard", CreatedBy: "developer@example.test", CPUMillis: 1000, MemoryMiB: 2048}, Policy: &policy, Usage: SchedulingUsage{ActiveTenantRuns: 1, ActiveUserRuns: 0, QueuedTenantRuns: 3, ActiveCPUMillis: 1000, ActiveMemoryMiB: 2048, BudgetUsedMinorUnits: 999}, Now: now}

	tests := []struct {
		name    string
		mutate  func(*EligibilityInput)
		outcome EligibilityOutcome
		code    string
	}{
		{name: "eligible at exact resource and queue boundaries", outcome: EligibilityEligible, code: EligibilityCodeEligible},
		{name: "tenant suspended", mutate: func(input *EligibilityInput) { input.TenantStatus = SchedulerResourceStatusSuspended }, outcome: EligibilityReject, code: EligibilityCodeTenantSuspended},
		{name: "project suspended", mutate: func(input *EligibilityInput) { input.ProjectStatus = SchedulerResourceStatusSuspended }, outcome: EligibilityReject, code: EligibilityCodeProjectSuspended},
		{name: "runtime unsupported", mutate: func(input *EligibilityInput) { input.Request.Runtime = "ruby-4" }, outcome: EligibilityReject, code: EligibilityCodeUnsupportedRuntime},
		{name: "profile unsupported", mutate: func(input *EligibilityInput) { input.Request.ExecutionProfile = "gpu" }, outcome: EligibilityReject, code: EligibilityCodeUnsupportedProfile},
		{name: "tenant concurrency", mutate: func(input *EligibilityInput) { input.Usage.ActiveTenantRuns = 2 }, outcome: EligibilityDefer, code: EligibilityCodeTenantConcurrency},
		{name: "user concurrency", mutate: func(input *EligibilityInput) { input.Usage.ActiveUserRuns = 1 }, outcome: EligibilityDefer, code: EligibilityCodeUserConcurrency},
		{name: "queue exceeded", mutate: func(input *EligibilityInput) { input.Usage.QueuedTenantRuns = 4 }, outcome: EligibilityDefer, code: EligibilityCodeQueuedRuns},
		{name: "cpu exceeded", mutate: func(input *EligibilityInput) { input.Usage.ActiveCPUMillis++ }, outcome: EligibilityDefer, code: EligibilityCodeCPU},
		{name: "memory exceeded", mutate: func(input *EligibilityInput) { input.Usage.ActiveMemoryMiB++ }, outcome: EligibilityDefer, code: EligibilityCodeMemory},
		{name: "budget exhausted", mutate: func(input *EligibilityInput) { input.Usage.BudgetUsedMinorUnits = 1000 }, outcome: EligibilityDefer, code: EligibilityCodeDailyBudget},
		{name: "policy missing", mutate: func(input *EligibilityInput) { input.Policy = nil }, outcome: EligibilityDefer, code: EligibilityCodePolicyMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			if test.mutate != nil {
				test.mutate(&input)
			}
			decision, err := EvaluateEligibility(input)
			if err != nil || decision.Outcome != test.outcome || decision.Code != test.code || decision.Explanation == "" {
				t.Fatalf("decision=%#v err=%v", decision, err)
			}
			if (decision.Outcome == EligibilityDefer) != (decision.NextEligibleAt != nil) {
				t.Fatalf("deferral timestamp mismatch: %#v", decision)
			}
		})
	}
}

func TestEligibilityRejectsUntrustedPolicyMetadata(t *testing.T) {
	now := time.Now().UTC()
	policy := SchedulingPolicy{MaxConcurrentRuns: 1, MaxUserConcurrentRuns: 1, MaxQueuedRuns: 1, MaxCPUMillis: 1000, MaxMemoryMiB: 1024, DailyBudgetMinorUnits: 1, AllowedRuntimes: []string{"python-3.12", "python-3.12"}, AllowedProfiles: []string{"standard"}, DeferralDuration: time.Minute}
	_, err := EvaluateEligibility(EligibilityInput{TenantStatus: SchedulerResourceStatusActive, ProjectStatus: SchedulerResourceStatusActive, Request: SchedulingRequest{Runtime: "python-3.12", ExecutionProfile: "standard", CreatedBy: "developer", CPUMillis: 1, MemoryMiB: 1}, Policy: &policy, Now: now})
	if err == nil {
		t.Fatal("duplicate policy metadata was accepted")
	}
}
