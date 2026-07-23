package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type SchedulingIntentRepository struct{ database *database.Pool }

func NewSchedulingIntentRepository(pool *database.Pool) *SchedulingIntentRepository {
	return &SchedulingIntentRepository{database: pool}
}

func (repository *SchedulingIntentRepository) Schedule(ctx context.Context, request ports.SchedulingIntentRequest) (ports.SchedulingIntentResult, error) {
	if err := validateSchedulingIntent(request); err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()

	run, leaseOwner, leaseExpiry, executionProfile, err := lockSchedulingRun(ctx, tx, request.Claim.TenantID, request.Claim.ID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if run.Status == domain.AgentRunProvisioning {
		return replaySchedulingIntent(ctx, tx, run, request)
	}
	if run.Status != domain.AgentRunScheduling || leaseOwner != request.LeaseOwner || leaseExpiry == nil || !leaseExpiry.After(request.Now.UTC()) || run.Version != request.Claim.Version || executionProfile != request.Claim.ExecutionProfile {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	if request.DesiredState.Runtime != run.Runtime || request.DesiredState.TaskReference != run.PromptReference ||
		request.DesiredState.TimeoutSeconds != run.TimeoutSeconds || request.DesiredState.MaxAttempts != run.MaxAttempts {
		return ports.SchedulingIntentResult{}, domain.ErrConflict
	}
	attempt, err := lockPendingAttempt(ctx, tx, run, request.AttemptID, request.AttemptVersion)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}

	claimed := request.Claim
	claimed.Version = run.Version
	claimed.ProjectID = run.ProjectID
	claimed.CreatedBy = run.CreatedBy
	claimed.Runtime = run.Runtime
	claimed.CPUMillis = run.CPUMillis
	claimed.MemoryMiB = run.MemoryMiB
	policy, err := readSchedulingPolicy(ctx, tx, run.TenantID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	var tenantStatus, projectStatus string
	if err := tx.QueryRowContext(ctx, `select tenant.status,project.status from tenants tenant join projects project on project.tenant_id=tenant.id where tenant.id=$1 and project.id=$2`, run.TenantID, run.ProjectID).Scan(&tenantStatus, &projectStatus); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	usage, err := readSchedulingUsage(ctx, tx, claimed, request.Now)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	decision, err := domain.EvaluateEligibility(domain.EligibilityInput{TenantStatus: tenantStatus, ProjectStatus: projectStatus, Request: domain.SchedulingRequest{Runtime: run.Runtime, ExecutionProfile: executionProfile, CreatedBy: run.CreatedBy, CPUMillis: run.CPUMillis, MemoryMiB: run.MemoryMiB}, Policy: policy, Usage: usage, Now: request.Now.UTC()})
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if decision.Outcome != domain.EligibilityEligible {
		return ports.SchedulingIntentResult{}, domain.EligibilityDeniedError{Decision: decision}
	}

	cluster, capacity, err := lockEligibleCluster(ctx, tx, run.TenantID, request.ClusterID, run.Runtime, executionProfile, request.Now, request.ClusterFreshness)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	var reservedCPU, reservedMemory int64
	if err := tx.QueryRowContext(ctx, `select coalesce(sum(cpu_millis),0),coalesce(sum(memory_mib),0) from capacity_reservations where cluster_id=$1 and state='RESERVED' and expires_at>$2`, cluster.ID, request.Now.UTC()).Scan(&reservedCPU, &reservedMemory); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if capacity.AllocatableCPUMillis-reservedCPU < int64(run.CPUMillis) || capacity.AllocatableMemoryMiB-reservedMemory < int64(run.MemoryMiB) {
		return ports.SchedulingIntentResult{}, domain.ErrCapacityUnavailable
	}
	usageDate := request.Now.UTC().Truncate(24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `insert into tenant_daily_budget_usage (tenant_id,usage_date,spent_minor_units,reserved_minor_units,updated_at) values ($1,$2,0,0,$3) on conflict do nothing`, run.TenantID, usageDate, request.Now.UTC()); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	var spent, budgetReserved int64
	if err := tx.QueryRowContext(ctx, `select spent_minor_units,reserved_minor_units from tenant_daily_budget_usage where tenant_id=$1 and usage_date=$2 for update`, run.TenantID, usageDate).Scan(&spent, &budgetReserved); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if policy == nil || request.BudgetMinorUnits > policy.DailyBudgetMinorUnits-spent-budgetReserved {
		return ports.SchedulingIntentResult{}, domain.ErrBudgetUnavailable
	}
	reservationRequest := ports.ReservationRequest{TenantID: run.TenantID, RunID: run.ID, AttemptID: attempt.ID, AttemptNumber: attempt.AttemptNumber, ClusterID: cluster.ID, CPUMillis: int64(run.CPUMillis), MemoryMiB: int64(run.MemoryMiB), BudgetMinorUnits: request.BudgetMinorUnits, Now: request.Now, TTL: request.ReservationTTL}
	bundle, err := newReservationBundle(reservationRequest, usageDate)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if err := insertReservationBundle(ctx, tx, bundle); err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `update tenant_daily_budget_usage set reserved_minor_units=reserved_minor_units+$3,updated_at=$4 where tenant_id=$1 and usage_date=$2`, run.TenantID, usageDate, request.BudgetMinorUnits, request.Now.UTC()); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}

	attempt.Status = domain.AttemptStarting
	attempt.Version++
	attempt.SelectedCluster = cluster.ID
	attempt.ExecutionProfile = executionProfile
	attempt.CapacityReservationID = bundle.Capacity.ID
	attempt.BudgetReservationID = bundle.Budget.ID
	attempt.UpdatedAt = request.Now.UTC()
	if _, err := tx.ExecContext(ctx, `update agent_run_attempts set status='STARTING',version=version+1,selected_cluster=$1,execution_profile=$2,capacity_reservation_id=$3,budget_reservation_id=$4,updated_at=$5 where tenant_id=$6 and id=$7 and version=$8`, attempt.SelectedCluster, attempt.ExecutionProfile, attempt.CapacityReservationID, attempt.BudgetReservationID, attempt.UpdatedAt, attempt.TenantID, attempt.ID, request.AttemptVersion); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	run.Status = domain.AgentRunProvisioning
	run.Version++
	run.UpdatedAt = request.Now.UTC()
	result, err := tx.ExecContext(ctx, `update agent_runs set status='PROVISIONING',version=version+1,updated_at=$1,scheduler_lease_owner=null,scheduler_lease_expires_at=null,scheduler_next_eligible_at=null,scheduler_decision_code='SCHEDULED',scheduler_decision_reason='cluster selected and resources reserved',scheduler_strategy=$2,scheduler_selection_score=$3 where tenant_id=$4 and id=$5 and version=$6`, run.UpdatedAt, request.Strategy, request.SelectionScore, run.TenantID, run.ID, request.Claim.Version)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	event, err := events.NewAgentRunScheduled(run, attempt, bundle.Capacity.ID, bundle.Budget.ID, request.CorrelationID, request.CausationID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if err := insertOutboxEvent(ctx, tx, event); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if err := insertHandoffIntent(ctx, tx, event.Envelope.EventID, run, attempt, request.DesiredState); err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	return ports.SchedulingIntentResult{RunVersion: run.Version, AttemptVersion: attempt.Version, CapacityReservationID: bundle.Capacity.ID, BudgetReservationID: bundle.Budget.ID, EventID: event.Envelope.EventID}, nil
}

func (repository *SchedulingIntentRepository) DeferCapacity(ctx context.Context, request ports.CapacityWaitRequest) (ports.SchedulingIntentResult, error) {
	if request.Claim.TenantID == uuid.Nil || request.Claim.ID == uuid.Nil || strings.TrimSpace(request.LeaseOwner) == "" || request.Claim.Version < 1 || request.Now.IsZero() || !request.NextEligibleAt.After(request.Now) || strings.TrimSpace(request.ReasonCode) == "" || len(request.ReasonCode) > 80 || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 500 || strings.TrimSpace(request.CorrelationID) == "" || strings.TrimSpace(request.CausationID) == "" {
		return ports.SchedulingIntentResult{}, fmt.Errorf("capacity-wait request is invalid")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	run, owner, expiry, _, err := lockSchedulingRun(ctx, tx, request.Claim.TenantID, request.Claim.ID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if run.Status == domain.AgentRunCapacityWait {
		var code, reason string
		var next time.Time
		if err := tx.QueryRowContext(ctx, `select scheduler_decision_code,scheduler_decision_reason,scheduler_next_eligible_at from agent_runs where tenant_id=$1 and id=$2`, run.TenantID, run.ID).Scan(&code, &reason, &next); err != nil {
			return ports.SchedulingIntentResult{}, translateError(err)
		}
		if run.Version == request.Claim.Version+1 && code == request.ReasonCode && reason == request.Reason && next.Equal(request.NextEligibleAt.UTC()) {
			var eventID uuid.UUID
			if err := tx.QueryRowContext(ctx, `select event_id from outbox_events where tenant_id=$1 and run_id=$2 and event_type=$3 and aggregate_version=$4`, run.TenantID, run.ID, events.AgentRunCapacityWaitType, run.Version).Scan(&eventID); err != nil {
				return ports.SchedulingIntentResult{}, translateError(err)
			}
			if err := tx.Commit(); err != nil {
				return ports.SchedulingIntentResult{}, translateError(err)
			}
			return ports.SchedulingIntentResult{RunVersion: run.Version, EventID: eventID, Replayed: true}, nil
		}
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	if run.Status != domain.AgentRunScheduling || owner != request.LeaseOwner || expiry == nil || !expiry.After(request.Now.UTC()) || run.Version != request.Claim.Version {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	run.Status = domain.AgentRunCapacityWait
	run.Version++
	run.UpdatedAt = request.Now.UTC()
	result, err := tx.ExecContext(ctx, `update agent_runs set status='CAPACITY_WAIT',version=version+1,updated_at=$1,scheduler_lease_owner=null,scheduler_lease_expires_at=null,scheduler_next_eligible_at=$2,scheduler_decision_code=$3,scheduler_decision_reason=$4 where tenant_id=$5 and id=$6 and version=$7`, run.UpdatedAt, request.NextEligibleAt.UTC(), request.ReasonCode, request.Reason, run.TenantID, run.ID, request.Claim.Version)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	event, err := events.NewAgentRunCapacityWait(run, request.ReasonCode, request.Reason, request.NextEligibleAt, request.CorrelationID, request.CausationID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if err := insertOutboxEvent(ctx, tx, event); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	return ports.SchedulingIntentResult{RunVersion: run.Version, EventID: event.Envelope.EventID}, nil
}

func (repository *SchedulingIntentRepository) Reject(ctx context.Context, request ports.SchedulingRejectRequest) (ports.SchedulingIntentResult, error) {
	if request.Claim.TenantID == uuid.Nil || request.Claim.ID == uuid.Nil || request.Claim.Version < 1 || strings.TrimSpace(request.LeaseOwner) == "" || request.Now.IsZero() || strings.TrimSpace(request.ReasonCode) == "" || len(request.ReasonCode) > 80 || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 500 || strings.TrimSpace(request.CorrelationID) == "" || strings.TrimSpace(request.CausationID) == "" {
		return ports.SchedulingIntentResult{}, fmt.Errorf("scheduling rejection request is invalid")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	run, owner, expiry, _, err := lockSchedulingRun(ctx, tx, request.Claim.TenantID, request.Claim.ID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if run.Status == domain.AgentRunPolicyRejected {
		var code, reason string
		if err := tx.QueryRowContext(ctx, `select scheduler_decision_code,scheduler_decision_reason from agent_runs where tenant_id=$1 and id=$2`, run.TenantID, run.ID).Scan(&code, &reason); err != nil {
			return ports.SchedulingIntentResult{}, translateError(err)
		}
		if run.Version != request.Claim.Version+1 || code != request.ReasonCode || reason != request.Reason {
			return ports.SchedulingIntentResult{}, domain.ErrConflict
		}
		var eventID uuid.UUID
		if err := tx.QueryRowContext(ctx, `select event_id from outbox_events where tenant_id=$1 and run_id=$2 and event_type=$3 and aggregate_version=$4`, run.TenantID, run.ID, events.AgentRunFailedType, run.Version).Scan(&eventID); err != nil {
			return ports.SchedulingIntentResult{}, translateError(err)
		}
		var attemptVersion int64
		if err := tx.QueryRowContext(ctx, `select version from agent_run_attempts where tenant_id=$1 and run_id=$2 and attempt_number=$3 and status='CANCELLED'`, run.TenantID, run.ID, run.AttemptCount).Scan(&attemptVersion); err != nil {
			return ports.SchedulingIntentResult{}, translateError(err)
		}
		if err := tx.Commit(); err != nil {
			return ports.SchedulingIntentResult{}, translateError(err)
		}
		return ports.SchedulingIntentResult{RunVersion: run.Version, AttemptVersion: attemptVersion, EventID: eventID, Replayed: true}, nil
	}
	if run.Status != domain.AgentRunScheduling || owner != request.LeaseOwner || expiry == nil || !expiry.After(request.Now.UTC()) || run.Version != request.Claim.Version {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	attempt, err := lockPendingAttempt(ctx, tx, run, request.Claim.AttemptID, request.Claim.AttemptVersion)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	completedAttempt := request.Now.UTC()
	result, err := tx.ExecContext(ctx, `update agent_run_attempts set status='CANCELLED',version=version+1,updated_at=$1,completed_at=$1 where tenant_id=$2 and id=$3 and version=$4`, completedAttempt, attempt.TenantID, attempt.ID, attempt.Version)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	run.Status = domain.AgentRunPolicyRejected
	run.FailureCategory = domain.FailurePolicy
	run.Version++
	run.UpdatedAt = request.Now.UTC()
	completed := run.UpdatedAt
	run.CompletedAt = &completed
	result, err = tx.ExecContext(ctx, `update agent_runs set status='POLICY_REJECTED',failure_category='POLICY',completed_at=$1,version=version+1,updated_at=$1,scheduler_lease_owner=null,scheduler_lease_expires_at=null,scheduler_next_eligible_at=null,scheduler_decision_code=$2,scheduler_decision_reason=$3 where tenant_id=$4 and id=$5 and version=$6`, run.UpdatedAt, request.ReasonCode, request.Reason, run.TenantID, run.ID, request.Claim.Version)
	if err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ports.SchedulingIntentResult{}, domain.ErrVersionConflict
	}
	event, err := events.NewAgentRunSchedulingRejected(run, request.ReasonCode, request.Reason, request.CorrelationID, request.CausationID)
	if err != nil {
		return ports.SchedulingIntentResult{}, err
	}
	if err := insertOutboxEvent(ctx, tx, event); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return ports.SchedulingIntentResult{}, translateError(err)
	}
	return ports.SchedulingIntentResult{RunVersion: run.Version, AttemptVersion: attempt.Version + 1, EventID: event.Envelope.EventID}, nil
}

func validateSchedulingIntent(request ports.SchedulingIntentRequest) error {
	if request.Claim.TenantID == uuid.Nil || request.Claim.ID == uuid.Nil || request.AttemptID == uuid.Nil || request.Claim.Version < 1 || request.AttemptVersion < 1 || strings.TrimSpace(request.LeaseOwner) == "" || request.ClusterID == "" || request.ClusterFreshness <= 0 || request.Strategy == "" || request.SelectionScore < 0 || request.BudgetMinorUnits < 0 || request.ReservationTTL <= 0 || request.ReservationTTL > 24*time.Hour || request.Now.IsZero() || strings.TrimSpace(request.CorrelationID) == "" || strings.TrimSpace(request.CausationID) == "" {
		return fmt.Errorf("scheduling intent request is invalid")
	}
	desired := request.DesiredState
	if desired.Runtime != request.Claim.Runtime || desired.ExecutionProfile != request.Claim.ExecutionProfile ||
		desired.TaskReference != request.Claim.PromptReference || desired.TimeoutSeconds != request.Claim.TimeoutSeconds ||
		desired.MaxAttempts != request.Claim.MaxAttempts || desired.CPUMillis != request.Claim.CPUMillis ||
		desired.MemoryMiB != request.Claim.MemoryMiB {
		return fmt.Errorf("scheduling desired state does not match claimed run")
	}
	if err := desired.Validate(request.Claim.AttemptNumber); err != nil {
		return err
	}
	return nil
}

func lockSchedulingRun(ctx context.Context, tx *sql.Tx, tenantID, runID uuid.UUID) (domain.AgentRun, string, *time.Time, string, error) {
	var run domain.AgentRun
	var owner, profile string
	var expiry *time.Time
	err := tx.QueryRowContext(ctx, `select id,tenant_id,project_id,prompt_reference,runtime,cpu_millis,memory_mib,timeout_seconds,max_attempts,attempt_count,status,version,created_by,updated_at,coalesce(scheduler_lease_owner,''),scheduler_lease_expires_at,execution_profile from agent_runs where tenant_id=$1 and id=$2 for update`, tenantID, runID).Scan(&run.ID, &run.TenantID, &run.ProjectID, &run.PromptReference, &run.Runtime, &run.CPUMillis, &run.MemoryMiB, &run.TimeoutSeconds, &run.MaxAttempts, &run.AttemptCount, &run.Status, &run.Version, &run.CreatedBy, &run.UpdatedAt, &owner, &expiry, &profile)
	return run, owner, expiry, profile, translateError(err)
}

func lockPendingAttempt(ctx context.Context, tx *sql.Tx, run domain.AgentRun, attemptID uuid.UUID, expectedVersion int64) (domain.AgentRunAttempt, error) {
	var attempt domain.AgentRunAttempt
	err := tx.QueryRowContext(ctx, `select id,tenant_id,run_id,attempt_number,status,version from agent_run_attempts where tenant_id=$1 and run_id=$2 and id=$3 and attempt_number=$4 for update`, run.TenantID, run.ID, attemptID, run.AttemptCount).Scan(&attempt.ID, &attempt.TenantID, &attempt.RunID, &attempt.AttemptNumber, &attempt.Status, &attempt.Version)
	if err != nil {
		return attempt, translateError(err)
	}
	if attempt.Status != domain.AttemptPending || attempt.Version != expectedVersion {
		return attempt, domain.ErrVersionConflict
	}
	return attempt, nil
}

func lockEligibleCluster(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, id, runtime, profile string, now time.Time, freshness time.Duration) (domain.ExecutionCluster, domain.ClusterCapacity, error) {
	var cluster domain.ExecutionCluster
	var capacity domain.ClusterCapacity
	var runtimes, profiles []byte
	err := tx.QueryRowContext(ctx, clusterSelect+` where cluster.id=$1 and cluster.status='ACTIVE' and not cluster.maintenance and cluster.last_heartbeat_at >= $2 and $3=any(cluster.supported_runtimes) and $4=any(cluster.supported_execution_profiles) and (not cluster.tenant_restricted or exists(select 1 from cluster_tenant_allowlist allowed where allowed.cluster_id=cluster.id and allowed.tenant_id=$5)) and capacity.observed_at >= $2 for update of cluster`, id, now.UTC().Add(-freshness), runtime, profile, tenantID).Scan(&cluster.ID, &cluster.Region, &cluster.Status, &runtimes, &profiles, &cluster.Maintenance, &cluster.TenantRestricted, &cluster.SchedulingWeight, &cluster.CPUCostMinorUnits, &cluster.MemoryGiBCostMinorUnits, &cluster.LastHeartbeatAt, &cluster.Version, &cluster.CreatedAt, &cluster.UpdatedAt, &capacity.ObservedAt, &capacity.AllocatableCPUMillis, &capacity.AllocatableMemoryMiB, &capacity.QueuedWorkloads)
	if errors.Is(err, sql.ErrNoRows) {
		return cluster, capacity, domain.ErrCapacityUnavailable
	}
	if err != nil {
		return cluster, capacity, translateError(err)
	}
	capacity.ClusterID = cluster.ID
	if err := json.Unmarshal(runtimes, &cluster.SupportedRuntimes); err != nil {
		return cluster, capacity, fmt.Errorf("decode cluster runtimes: %w", err)
	}
	if err := json.Unmarshal(profiles, &cluster.SupportedExecutionProfiles); err != nil {
		return cluster, capacity, fmt.Errorf("decode cluster profiles: %w", err)
	}
	if err := cluster.Validate(now); err != nil {
		return cluster, capacity, domain.ErrCapacityUnavailable
	}
	if err := capacity.Validate(now); err != nil {
		return cluster, capacity, domain.ErrCapacityUnavailable
	}
	return cluster, capacity, nil
}

func insertReservationBundle(ctx context.Context, tx *sql.Tx, bundle domain.ReservationBundle) error {
	if _, err := tx.ExecContext(ctx, `insert into capacity_reservations (id,tenant_id,run_id,attempt_id,attempt_number,cluster_id,cpu_millis,memory_mib,state,expires_at,created_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, bundle.Capacity.ID, bundle.Capacity.TenantID, bundle.Capacity.RunID, bundle.Capacity.AttemptID, bundle.Capacity.AttemptNumber, bundle.Capacity.ClusterID, bundle.Capacity.CPUMillis, bundle.Capacity.MemoryMiB, bundle.Capacity.State, bundle.Capacity.ExpiresAt, bundle.Capacity.CreatedAt); err != nil {
		return translateError(err)
	}
	if _, err := tx.ExecContext(ctx, `insert into budget_reservations (id,capacity_reservation_id,tenant_id,run_id,attempt_number,usage_date,amount_minor_units,state,expires_at,created_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, bundle.Budget.ID, bundle.Budget.CapacityReservationID, bundle.Budget.TenantID, bundle.Budget.RunID, bundle.Budget.AttemptNumber, bundle.Budget.UsageDate, bundle.Budget.AmountMinorUnits, bundle.Budget.State, bundle.Budget.ExpiresAt, bundle.Budget.CreatedAt); err != nil {
		return translateError(err)
	}
	return nil
}

func replaySchedulingIntent(ctx context.Context, tx *sql.Tx, run domain.AgentRun, request ports.SchedulingIntentRequest) (ports.SchedulingIntentResult, error) {
	var result ports.SchedulingIntentResult
	var cluster, profile string
	var cpu, memory, budget int64
	var createdAt, expiresAt time.Time
	err := tx.QueryRowContext(ctx, `select attempt.version,attempt.selected_cluster,attempt.execution_profile,
		attempt.capacity_reservation_id,attempt.budget_reservation_id,capacity.cpu_millis,
		capacity.memory_mib,budget.amount_minor_units,capacity.created_at,capacity.expires_at
		from agent_run_attempts attempt
		join capacity_reservations capacity on capacity.id=attempt.capacity_reservation_id
		join budget_reservations budget on budget.id=attempt.budget_reservation_id
		where attempt.tenant_id=$1 and attempt.run_id=$2 and attempt.attempt_number=$3`, run.TenantID, run.ID, run.AttemptCount).Scan(&result.AttemptVersion, &cluster, &profile, &result.CapacityReservationID, &result.BudgetReservationID, &cpu, &memory, &budget, &createdAt, &expiresAt)
	if err != nil {
		return result, translateError(err)
	}
	var strategy string
	var score int64
	if err := tx.QueryRowContext(ctx, `select scheduler_strategy,scheduler_selection_score from agent_runs where tenant_id=$1 and id=$2`, run.TenantID, run.ID).Scan(&strategy, &score); err != nil {
		return result, translateError(err)
	}
	if cluster != request.ClusterID || profile != request.Claim.ExecutionProfile || cpu != int64(run.CPUMillis) || memory != int64(run.MemoryMiB) || budget != request.BudgetMinorUnits || expiresAt.Sub(createdAt) != request.ReservationTTL || strategy != request.Strategy || score != request.SelectionScore {
		return result, domain.ErrConflict
	}
	if err := tx.QueryRowContext(ctx, `select event_id from outbox_events where tenant_id=$1 and run_id=$2 and event_type=$3 and aggregate_version=$4`, run.TenantID, run.ID, events.AgentRunScheduledType, run.Version).Scan(&result.EventID); err != nil {
		return result, translateError(err)
	}
	var serialized []byte
	if err := tx.QueryRowContext(ctx, `select desired_state from agentrun_handoff_intents where event_id=$1 and tenant_id=$2 and run_id=$3 and attempt_number=$4`, result.EventID, run.TenantID, run.ID, run.AttemptCount).Scan(&serialized); err != nil {
		return result, translateError(err)
	}
	var stored domain.AgentRunDesiredState
	if err := json.Unmarshal(serialized, &stored); err != nil || !desiredStateMatches(stored, request.DesiredState) {
		return result, domain.ErrConflict
	}
	result.RunVersion = run.Version
	result.Replayed = true
	if err := tx.Commit(); err != nil {
		return result, translateError(err)
	}
	return result, nil
}

func insertHandoffIntent(ctx context.Context, tx *sql.Tx, eventID uuid.UUID, run domain.AgentRun, attempt domain.AgentRunAttempt, desired domain.AgentRunDesiredState) error {
	serialized, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("serialize AgentRun handoff intent: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert into agentrun_handoff_intents (event_id,tenant_id,run_id,attempt_id,attempt_number,cluster_id,desired_state,created_at) values ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, eventID, run.TenantID, run.ID, attempt.ID, attempt.AttemptNumber, attempt.SelectedCluster, serialized, run.UpdatedAt); err != nil {
		return translateError(err)
	}
	return nil
}

func desiredStateMatches(stored, desired domain.AgentRunDesiredState) bool {
	serializedStored, err := json.Marshal(stored)
	if err != nil {
		return false
	}
	serializedExpected, err := json.Marshal(desired)
	return err == nil && string(serializedStored) == string(serializedExpected)
}

var _ ports.SchedulingIntentRepository = (*SchedulingIntentRepository)(nil)
