package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type SchedulerEligibilityRepository struct{ database *database.Pool }

func NewSchedulerEligibilityRepository(pool *database.Pool) *SchedulerEligibilityRepository {
	return &SchedulerEligibilityRepository{database: pool}
}

func (repository *SchedulerEligibilityRepository) Evaluate(ctx context.Context, run ports.ClaimedRun, now time.Time) (domain.EligibilityDecision, error) {
	if run.ID.Version() != 7 || run.TenantID.Version() != 7 || run.ProjectID.Version() != 7 || run.AttemptNumber < 1 || run.Version < 1 || now.IsZero() {
		return domain.EligibilityDecision{}, fmt.Errorf("claimed run identity and evaluation time are required")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return domain.EligibilityDecision{}, fmt.Errorf("begin eligibility evaluation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var tenantStatus, projectStatus string
	if err := tx.QueryRowContext(ctx, `
		select tenant.status, project.status
		from tenants tenant join projects project on project.tenant_id = tenant.id
		where tenant.id = $1 and project.id = $2`, run.TenantID, run.ProjectID).Scan(&tenantStatus, &projectStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EligibilityDecision{}, domain.ErrNotFound
		}
		return domain.EligibilityDecision{}, fmt.Errorf("read scheduling ownership status: %w", err)
	}

	policy, err := readSchedulingPolicy(ctx, tx, run.TenantID)
	if err != nil {
		return domain.EligibilityDecision{}, err
	}
	usage, err := readSchedulingUsage(ctx, tx, run, now)
	if err != nil {
		return domain.EligibilityDecision{}, err
	}
	decision, err := domain.EvaluateEligibility(domain.EligibilityInput{
		TenantStatus: tenantStatus, ProjectStatus: projectStatus,
		Request: domain.SchedulingRequest{Runtime: run.Runtime, ExecutionProfile: run.ExecutionProfile, CreatedBy: run.CreatedBy, CPUMillis: run.CPUMillis, MemoryMiB: run.MemoryMiB},
		Policy:  policy, Usage: usage, Now: now.UTC(),
	})
	if err != nil {
		return domain.EligibilityDecision{}, err
	}
	decisionID, err := uuid.NewV7()
	if err != nil {
		return domain.EligibilityDecision{}, fmt.Errorf("generate eligibility decision ID: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		insert into scheduler_eligibility_decisions (
			id, tenant_id, run_id, attempt_number, run_version, outcome, reason_code,
			explanation, active_tenant_runs, active_user_runs, queued_tenant_runs,
			active_cpu_millis, active_memory_mib, budget_used_minor_units,
			next_eligible_at, evaluated_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		decisionID, run.TenantID, run.ID, run.AttemptNumber, run.Version,
		decision.Outcome, decision.Code, decision.Explanation, usage.ActiveTenantRuns,
		usage.ActiveUserRuns, usage.QueuedTenantRuns, usage.ActiveCPUMillis,
		usage.ActiveMemoryMiB, usage.BudgetUsedMinorUnits, decision.NextEligibleAt,
		now.UTC())
	if err != nil {
		return domain.EligibilityDecision{}, fmt.Errorf("persist eligibility decision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.EligibilityDecision{}, fmt.Errorf("commit eligibility decision: %w", err)
	}
	return decision, nil
}

func readSchedulingPolicy(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID) (*domain.SchedulingPolicy, error) {
	var policy domain.SchedulingPolicy
	var runtimesJSON, profilesJSON []byte
	var deferralSeconds int
	err := tx.QueryRowContext(ctx, `
		select max_concurrent_runs, max_user_concurrent_runs, max_queued_runs,
			max_cpu_millis, max_memory_mib, daily_budget_minor_units,
			array_to_json(allowed_runtimes), array_to_json(allowed_execution_profiles),
			deferral_seconds
		from scheduler_tenant_policies where tenant_id = $1
		for update`, tenantID).Scan(&policy.MaxConcurrentRuns, &policy.MaxUserConcurrentRuns, &policy.MaxQueuedRuns, &policy.MaxCPUMillis, &policy.MaxMemoryMiB, &policy.DailyBudgetMinorUnits, &runtimesJSON, &profilesJSON, &deferralSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tenant scheduling policy: %w", err)
	}
	if err := json.Unmarshal(runtimesJSON, &policy.AllowedRuntimes); err != nil {
		return nil, fmt.Errorf("decode allowed runtimes: %w", err)
	}
	if err := json.Unmarshal(profilesJSON, &policy.AllowedProfiles); err != nil {
		return nil, fmt.Errorf("decode allowed profiles: %w", err)
	}
	policy.DeferralDuration = time.Duration(deferralSeconds) * time.Second
	return &policy, nil
}

func readSchedulingUsage(ctx context.Context, tx *sql.Tx, run ports.ClaimedRun, now time.Time) (domain.SchedulingUsage, error) {
	var usage domain.SchedulingUsage
	err := tx.QueryRowContext(ctx, `
		select
			count(*) filter (where status in ('PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING')),
			count(*) filter (where created_by = $2 and status in ('PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING')),
			count(*) filter (where status in ('QUEUED', 'SCHEDULING', 'CAPACITY_WAIT')),
			coalesce(sum(cpu_millis) filter (where status in ('PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING')), 0),
			coalesce(sum(memory_mib) filter (where status in ('PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING')), 0)
		from agent_runs where tenant_id = $1`, run.TenantID, run.CreatedBy).Scan(&usage.ActiveTenantRuns, &usage.ActiveUserRuns, &usage.QueuedTenantRuns, &usage.ActiveCPUMillis, &usage.ActiveMemoryMiB)
	if err != nil {
		return domain.SchedulingUsage{}, fmt.Errorf("aggregate tenant scheduling usage: %w", err)
	}
	err = tx.QueryRowContext(ctx, `
		select coalesce(spent_minor_units + reserved_minor_units, 0)
		from tenant_daily_budget_usage where tenant_id = $1 and usage_date = $2`, run.TenantID, now.UTC().Format("2006-01-02")).Scan(&usage.BudgetUsedMinorUnits)
	if errors.Is(err, sql.ErrNoRows) {
		usage.BudgetUsedMinorUnits = 0
		return usage, nil
	}
	if err != nil {
		return domain.SchedulingUsage{}, fmt.Errorf("read tenant budget usage: %w", err)
	}
	return usage, nil
}

var _ ports.SchedulerEligibilityRepository = (*SchedulerEligibilityRepository)(nil)
