package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type EligibilityOutcome string

const (
	EligibilityEligible EligibilityOutcome = "ELIGIBLE"
	EligibilityDefer    EligibilityOutcome = "DEFER"
	EligibilityReject   EligibilityOutcome = "REJECT"
)

const (
	EligibilityCodeEligible           = "ELIGIBLE"
	EligibilityCodePolicyMissing      = "POLICY_MISSING"
	EligibilityCodeTenantSuspended    = "TENANT_SUSPENDED"
	EligibilityCodeProjectSuspended   = "PROJECT_SUSPENDED"
	EligibilityCodeUnsupportedRuntime = "UNSUPPORTED_RUNTIME"
	EligibilityCodeUnsupportedProfile = "UNSUPPORTED_PROFILE"
	EligibilityCodeTenantConcurrency  = "TENANT_CONCURRENCY_LIMIT"
	EligibilityCodeUserConcurrency    = "USER_CONCURRENCY_LIMIT"
	EligibilityCodeQueuedRuns         = "TENANT_QUEUE_LIMIT"
	EligibilityCodeCPU                = "TENANT_CPU_LIMIT"
	EligibilityCodeMemory             = "TENANT_MEMORY_LIMIT"
	EligibilityCodeDailyBudget        = "DAILY_BUDGET_LIMIT"
	SchedulerResourceStatusActive     = "ACTIVE"
	SchedulerResourceStatusSuspended  = "SUSPENDED"
)

type SchedulingRequest struct {
	Runtime          string
	ExecutionProfile string
	CreatedBy        string
	CPUMillis        int
	MemoryMiB        int
}

type SchedulingPolicy struct {
	MaxConcurrentRuns     int
	MaxUserConcurrentRuns int
	MaxQueuedRuns         int
	MaxCPUMillis          int64
	MaxMemoryMiB          int64
	DailyBudgetMinorUnits int64
	AllowedRuntimes       []string
	AllowedProfiles       []string
	DeferralDuration      time.Duration
}

type SchedulingUsage struct {
	ActiveTenantRuns     int
	ActiveUserRuns       int
	QueuedTenantRuns     int
	ActiveCPUMillis      int64
	ActiveMemoryMiB      int64
	BudgetUsedMinorUnits int64
}

type EligibilityInput struct {
	TenantStatus  string
	ProjectStatus string
	Request       SchedulingRequest
	Policy        *SchedulingPolicy
	Usage         SchedulingUsage
	Now           time.Time
}

type EligibilityDecision struct {
	Outcome        EligibilityOutcome
	Code           string
	Explanation    string
	NextEligibleAt *time.Time
}

// EligibilityDeniedError reports a changed policy snapshot during final admission.
type EligibilityDeniedError struct{ Decision EligibilityDecision }

func (err EligibilityDeniedError) Error() string {
	return "scheduling eligibility changed: " + err.Decision.Code
}

type eligibilityRule func(EligibilityInput) *EligibilityDecision

var schedulerPolicyValuePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,119}$`)

var eligibilityRules = []eligibilityRule{
	resourceStatusRule,
	runtimeProfileRule,
	concurrencyRule,
	queueRule,
	resourceQuotaRule,
	budgetRule,
}

// EvaluateEligibility applies focused, ordered policy specifications and
// explains the first blocking decision.
func EvaluateEligibility(input EligibilityInput) (EligibilityDecision, error) {
	input.TenantStatus = strings.TrimSpace(input.TenantStatus)
	input.ProjectStatus = strings.TrimSpace(input.ProjectStatus)
	input.Request.Runtime = strings.TrimSpace(input.Request.Runtime)
	input.Request.ExecutionProfile = strings.TrimSpace(input.Request.ExecutionProfile)
	input.Request.CreatedBy = strings.TrimSpace(input.Request.CreatedBy)
	if input.Now.IsZero() || input.Request.Runtime == "" || input.Request.ExecutionProfile == "" || input.Request.CreatedBy == "" || input.Request.CPUMillis < 1 || input.Request.MemoryMiB < 1 {
		return EligibilityDecision{}, fmt.Errorf("scheduling request metadata is invalid")
	}
	if input.Policy == nil {
		return deferredDecision(EligibilityCodePolicyMissing, "tenant scheduling policy is not configured", input.Now, 30*time.Second), nil
	}
	if err := validateSchedulingPolicy(*input.Policy); err != nil {
		return EligibilityDecision{}, err
	}
	if input.Usage.ActiveTenantRuns < 0 || input.Usage.ActiveUserRuns < 0 || input.Usage.QueuedTenantRuns < 0 || input.Usage.ActiveCPUMillis < 0 || input.Usage.ActiveMemoryMiB < 0 || input.Usage.BudgetUsedMinorUnits < 0 {
		return EligibilityDecision{}, fmt.Errorf("scheduling usage cannot be negative")
	}
	for _, rule := range eligibilityRules {
		if decision := rule(input); decision != nil {
			return *decision, nil
		}
	}
	return EligibilityDecision{Outcome: EligibilityEligible, Code: EligibilityCodeEligible, Explanation: "run satisfies scheduling policy"}, nil
}

func validateSchedulingPolicy(policy SchedulingPolicy) error {
	if policy.MaxConcurrentRuns < 1 || policy.MaxUserConcurrentRuns < 1 || policy.MaxUserConcurrentRuns > policy.MaxConcurrentRuns || policy.MaxQueuedRuns < 1 || policy.MaxCPUMillis < 1 || policy.MaxMemoryMiB < 1 || policy.DailyBudgetMinorUnits < 0 || len(policy.AllowedRuntimes) == 0 || len(policy.AllowedProfiles) == 0 || policy.DeferralDuration <= 0 {
		return fmt.Errorf("scheduling policy is invalid")
	}
	if !validPolicyValues(policy.AllowedRuntimes) || !validPolicyValues(policy.AllowedProfiles) {
		return fmt.Errorf("scheduling policy runtime and profile values must be normalized and unique")
	}
	return nil
}

func validPolicyValues(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !schedulerPolicyValuePattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func resourceStatusRule(input EligibilityInput) *EligibilityDecision {
	if input.TenantStatus != SchedulerResourceStatusActive {
		return &EligibilityDecision{Outcome: EligibilityReject, Code: EligibilityCodeTenantSuspended, Explanation: "tenant is not active"}
	}
	if input.ProjectStatus != SchedulerResourceStatusActive {
		return &EligibilityDecision{Outcome: EligibilityReject, Code: EligibilityCodeProjectSuspended, Explanation: "project is not active"}
	}
	return nil
}

func runtimeProfileRule(input EligibilityInput) *EligibilityDecision {
	if !slices.Contains(input.Policy.AllowedRuntimes, input.Request.Runtime) {
		return &EligibilityDecision{Outcome: EligibilityReject, Code: EligibilityCodeUnsupportedRuntime, Explanation: "runtime is not allowed by tenant policy"}
	}
	if !slices.Contains(input.Policy.AllowedProfiles, input.Request.ExecutionProfile) {
		return &EligibilityDecision{Outcome: EligibilityReject, Code: EligibilityCodeUnsupportedProfile, Explanation: "execution profile is not allowed by tenant policy"}
	}
	return nil
}

func concurrencyRule(input EligibilityInput) *EligibilityDecision {
	if input.Usage.ActiveTenantRuns >= input.Policy.MaxConcurrentRuns {
		decision := deferredDecision(EligibilityCodeTenantConcurrency, "tenant concurrent-run limit is reached", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	if input.Usage.ActiveUserRuns >= input.Policy.MaxUserConcurrentRuns {
		decision := deferredDecision(EligibilityCodeUserConcurrency, "user concurrent-run limit is reached", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	return nil
}

func queueRule(input EligibilityInput) *EligibilityDecision {
	if input.Usage.QueuedTenantRuns > input.Policy.MaxQueuedRuns {
		decision := deferredDecision(EligibilityCodeQueuedRuns, "tenant queued-run limit is exceeded", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	return nil
}

func resourceQuotaRule(input EligibilityInput) *EligibilityDecision {
	if input.Usage.ActiveCPUMillis+int64(input.Request.CPUMillis) > input.Policy.MaxCPUMillis {
		decision := deferredDecision(EligibilityCodeCPU, "tenant CPU limit has insufficient headroom", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	if input.Usage.ActiveMemoryMiB+int64(input.Request.MemoryMiB) > input.Policy.MaxMemoryMiB {
		decision := deferredDecision(EligibilityCodeMemory, "tenant memory limit has insufficient headroom", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	return nil
}

func budgetRule(input EligibilityInput) *EligibilityDecision {
	if input.Usage.BudgetUsedMinorUnits >= input.Policy.DailyBudgetMinorUnits {
		decision := deferredDecision(EligibilityCodeDailyBudget, "tenant daily budget has no available balance", input.Now, input.Policy.DeferralDuration)
		return &decision
	}
	return nil
}

func deferredDecision(code, explanation string, now time.Time, delay time.Duration) EligibilityDecision {
	next := now.UTC().Add(delay)
	return EligibilityDecision{Outcome: EligibilityDefer, Code: code, Explanation: explanation, NextEligibleAt: &next}
}
