package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// AgentRunRepository is the PostgreSQL adapter for tenant-scoped runs and attempts.
type AgentRunRepository struct {
	database *database.Pool
}

// NewAgentRunRepository constructs the tenant-scoped adapter.
func NewAgentRunRepository(pool *database.Pool) *AgentRunRepository {
	return &AgentRunRepository{database: pool}
}

// Create persists the accepted queued run and its first pending attempt atomically.
func (repository *AgentRunRepository) Create(ctx context.Context, run domain.AgentRun, attempt domain.AgentRunAttempt) error {
	if err := validateInitialAttempt(run, attempt); err != nil {
		return err
	}
	event, err := events.NewAgentRunRequested(run, attempt)
	if err != nil {
		return err
	}
	tx, err := repository.database.BeginTenant(ctx, run.TenantID)
	if err != nil {
		return translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		insert into agent_runs (id, tenant_id, project_id, prompt_reference, runtime, cpu_millis, memory_mib, timeout_seconds, max_attempts, attempt_count, status, failure_category, idempotency_key, request_hash, version, created_by, cancellation_reason, cancellation_requested_by, cancellation_requested_at, created_at, updated_at, completed_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, nullif($12, ''), $13, $14, $15, $16, nullif($17, ''), nullif($18, ''), $19, $20, $21, $22)`,
		run.ID, run.TenantID, run.ProjectID, run.PromptReference, run.Runtime, run.CPUMillis, run.MemoryMiB, run.TimeoutSeconds, run.MaxAttempts, run.AttemptCount, run.Status, run.FailureCategory, run.IdempotencyKey, run.RequestHash, run.Version, run.CreatedBy, run.CancellationReason, run.CancellationRequestedBy, run.CancellationRequestedAt, run.CreatedAt, run.UpdatedAt, run.CompletedAt)
	if err != nil {
		return translateError(err)
	}
	if err := insertAttempt(ctx, tx, attempt); err != nil {
		return translateError(err)
	}
	if err := insertOutboxEvent(ctx, tx, event); err != nil {
		return translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return translateError(err)
	}
	return nil
}

// Get returns a run only when it belongs to the caller's tenant.
func (repository *AgentRunRepository) Get(ctx context.Context, tenantID, runID uuid.UUID) (domain.AgentRun, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanAgentRun(tx.QueryRowContext(ctx, agentRunSelect+` where tenant_id = $1 and id = $2`, tenantID, runID))
	if err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	return run, nil
}

// List returns a tenant-scoped, keyset-paginated run history.
func (repository *AgentRunRepository) List(ctx context.Context, tenantID uuid.UUID, page ports.AgentRunPage) ([]domain.AgentRun, error) {
	if (page.AfterCreatedAt == nil) != (page.AfterID == nil) {
		return nil, fmt.Errorf("run page cursor must include creation time and ID")
	}
	if page.Limit <= 0 || page.Limit > 100 {
		page.Limit = 50
	}
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, agentRunSelect+`
		where tenant_id = $1
		  and ($2::uuid is null or project_id = $2)
		  and ($3::timestamptz is null or (created_at, id) < ($3, $4))
		order by created_at desc, id desc
		limit $5`, tenantID, page.ProjectID, page.AfterCreatedAt, page.AfterID, page.Limit)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]domain.AgentRun, 0, page.Limit)
	for rows.Next() {
		run, scanErr := scanAgentRun(rows)
		if scanErr != nil {
			return nil, translateError(scanErr)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, translateError(err)
	}
	return runs, nil
}

// Save applies a validated transition only when the expected aggregate version still matches.
func (repository *AgentRunRepository) Save(ctx context.Context, run domain.AgentRun, expectedVersion int64) (domain.AgentRun, error) {
	tx, err := repository.database.BeginTenant(ctx, run.TenantID)
	if err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := updateRun(ctx, tx, run, expectedVersion, false)
	if err != nil {
		return domain.AgentRun{}, repository.saveError(ctx, tx, run.TenantID, run.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	return updated, nil
}

// CreateNextAttempt atomically records the next monotonic pending attempt and run version.
func (repository *AgentRunRepository) CreateNextAttempt(ctx context.Context, run domain.AgentRun, attempt domain.AgentRunAttempt, expectedVersion int64) (domain.AgentRun, error) {
	if attempt.TenantID != run.TenantID || attempt.RunID != run.ID || attempt.Status != domain.AttemptPending || attempt.AttemptNumber != run.AttemptCount {
		return domain.AgentRun{}, fmt.Errorf("next attempt does not match run state")
	}
	tx, err := repository.database.BeginTenant(ctx, run.TenantID)
	if err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := updateRun(ctx, tx, run, expectedVersion, true)
	if err != nil {
		return domain.AgentRun{}, repository.saveError(ctx, tx, run.TenantID, run.ID, err)
	}
	if err := insertAttempt(ctx, tx, attempt); err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRun{}, translateError(err)
	}
	return updated, nil
}

// GetAttempt returns one attempt only inside the supplied tenant scope.
func (repository *AgentRunRepository) GetAttempt(ctx context.Context, tenantID, runID uuid.UUID, number int) (domain.AgentRunAttempt, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return domain.AgentRunAttempt{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, attemptSelect+` where tenant_id = $1 and run_id = $2 and attempt_number = $3`, tenantID, runID, number))
	if err != nil {
		return domain.AgentRunAttempt{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRunAttempt{}, translateError(err)
	}
	return attempt, nil
}

// ListAttempts returns immutable attempt history ordered by attempt number.
func (repository *AgentRunRepository) ListAttempts(ctx context.Context, tenantID, runID uuid.UUID) ([]domain.AgentRunAttempt, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, attemptSelect+` where tenant_id = $1 and run_id = $2 order by attempt_number asc`, tenantID, runID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()
	attempts := make([]domain.AgentRunAttempt, 0)
	for rows.Next() {
		attempt, scanErr := scanAttempt(rows)
		if scanErr != nil {
			return nil, translateError(scanErr)
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, translateError(err)
	}
	return attempts, nil
}

// SaveAttempt applies a validated attempt transition with optimistic locking.
func (repository *AgentRunRepository) SaveAttempt(ctx context.Context, attempt domain.AgentRunAttempt, expectedVersion int64) (domain.AgentRunAttempt, error) {
	tx, err := repository.database.BeginTenant(ctx, attempt.TenantID)
	if err != nil {
		return domain.AgentRunAttempt{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := scanAttempt(tx.QueryRowContext(ctx, `
		update agent_run_attempts
		set status = $1, failure_category = nullif($2, ''), selected_cluster = nullif($3, ''), execution_profile = nullif($4, ''),
			capacity_reservation_id = nullif($5::uuid, '00000000-0000-0000-0000-000000000000'), budget_reservation_id = nullif($6::uuid, '00000000-0000-0000-0000-000000000000'),
			workload_reference = nullif($7, ''), version = version + 1, updated_at = $8, started_at = $9, completed_at = $10
		where tenant_id = $11 and id = $12 and version = $13
		returning id, tenant_id, run_id, attempt_number, status, coalesce(failure_category, ''), version, coalesce(selected_cluster, ''), coalesce(execution_profile, ''),
			coalesce(capacity_reservation_id, '00000000-0000-0000-0000-000000000000'), coalesce(budget_reservation_id, '00000000-0000-0000-0000-000000000000'),
			coalesce(workload_reference, ''), created_at, updated_at, started_at, completed_at`,
		attempt.Status, attempt.FailureCategory, attempt.SelectedCluster, attempt.ExecutionProfile, attempt.CapacityReservationID, attempt.BudgetReservationID,
		attempt.WorkloadReference, attempt.UpdatedAt, attempt.StartedAt, attempt.CompletedAt, attempt.TenantID, attempt.ID, expectedVersion))
	if err != nil {
		return domain.AgentRunAttempt{}, repository.saveError(ctx, tx, attempt.TenantID, attempt.RunID, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentRunAttempt{}, translateError(err)
	}
	return updated, nil
}

func (repository *AgentRunRepository) saveError(ctx context.Context, tx *database.TenantTx, tenantID, runID uuid.UUID, cause error) error {
	if !errors.Is(cause, sql.ErrNoRows) {
		return translateError(cause)
	}
	var exists bool
	err := tx.QueryRowContext(ctx, `select exists(select 1 from agent_runs where tenant_id = $1 and id = $2)`, tenantID, runID).Scan(&exists)
	if err != nil {
		return translateError(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return domain.ErrVersionConflict
}

func updateRun(ctx context.Context, tx *database.TenantTx, run domain.AgentRun, expectedVersion int64, requireAttemptIncrease bool) (domain.AgentRun, error) {
	return scanAgentRun(tx.QueryRowContext(ctx, `
		update agent_runs
		set status = $1, failure_category = nullif($2, ''), attempt_count = $3, version = version + 1, updated_at = $4,
			cancellation_reason = nullif($5, ''), cancellation_requested_by = nullif($6, ''), cancellation_requested_at = $7, completed_at = $8
		where tenant_id = $9 and id = $10 and version = $11
		  and ($12::boolean = false or attempt_count + 1 = $3)
		returning id, tenant_id, project_id, prompt_reference, runtime, cpu_millis, memory_mib, timeout_seconds, max_attempts, attempt_count, status,
			coalesce(failure_category, ''), idempotency_key, request_hash, version, created_by, coalesce(cancellation_reason, ''),
			coalesce(cancellation_requested_by, ''), cancellation_requested_at, created_at, updated_at, completed_at`,
		run.Status, run.FailureCategory, run.AttemptCount, run.UpdatedAt, run.CancellationReason, run.CancellationRequestedBy, run.CancellationRequestedAt, run.CompletedAt,
		run.TenantID, run.ID, expectedVersion, requireAttemptIncrease))
}

func insertAttempt(ctx context.Context, tx *database.TenantTx, attempt domain.AgentRunAttempt) error {
	_, err := tx.ExecContext(ctx, `
		insert into agent_run_attempts (id, tenant_id, run_id, attempt_number, status, failure_category, version, selected_cluster, workload_reference, created_at, updated_at, started_at, completed_at)
		values ($1, $2, $3, $4, $5, nullif($6, ''), $7, nullif($8, ''), nullif($9, ''), $10, $11, $12, $13)`,
		attempt.ID, attempt.TenantID, attempt.RunID, attempt.AttemptNumber, attempt.Status, attempt.FailureCategory, attempt.Version, attempt.SelectedCluster, attempt.WorkloadReference, attempt.CreatedAt, attempt.UpdatedAt, attempt.StartedAt, attempt.CompletedAt)
	return err
}

func validateInitialAttempt(run domain.AgentRun, attempt domain.AgentRunAttempt) error {
	if run.Status != domain.AgentRunQueued || run.AttemptCount != 1 || attempt.TenantID != run.TenantID || attempt.RunID != run.ID || attempt.AttemptNumber != 1 || attempt.Status != domain.AttemptPending {
		return fmt.Errorf("initial run and attempt state is invalid")
	}
	return nil
}

type scanner interface{ Scan(...any) error }

const agentRunSelect = `select id, tenant_id, project_id, prompt_reference, runtime, cpu_millis, memory_mib, timeout_seconds, max_attempts, attempt_count, status,
	coalesce(failure_category, ''), idempotency_key, request_hash, version, created_by, coalesce(cancellation_reason, ''),
	coalesce(cancellation_requested_by, ''), cancellation_requested_at, created_at, updated_at, completed_at from agent_runs`

const attemptSelect = `select id, tenant_id, run_id, attempt_number, status, coalesce(failure_category, ''), version, coalesce(selected_cluster, ''),
	coalesce(execution_profile, ''), coalesce(capacity_reservation_id, '00000000-0000-0000-0000-000000000000'),
	coalesce(budget_reservation_id, '00000000-0000-0000-0000-000000000000'), coalesce(workload_reference, ''), created_at, updated_at, started_at, completed_at from agent_run_attempts`

func scanAgentRun(row scanner) (domain.AgentRun, error) {
	var run domain.AgentRun
	err := row.Scan(&run.ID, &run.TenantID, &run.ProjectID, &run.PromptReference, &run.Runtime, &run.CPUMillis, &run.MemoryMiB, &run.TimeoutSeconds, &run.MaxAttempts, &run.AttemptCount, &run.Status,
		&run.FailureCategory, &run.IdempotencyKey, &run.RequestHash, &run.Version, &run.CreatedBy, &run.CancellationReason, &run.CancellationRequestedBy,
		&run.CancellationRequestedAt, &run.CreatedAt, &run.UpdatedAt, &run.CompletedAt)
	return run, err
}

func scanAttempt(row scanner) (domain.AgentRunAttempt, error) {
	var attempt domain.AgentRunAttempt
	err := row.Scan(&attempt.ID, &attempt.TenantID, &attempt.RunID, &attempt.AttemptNumber, &attempt.Status, &attempt.FailureCategory, &attempt.Version,
		&attempt.SelectedCluster, &attempt.ExecutionProfile, &attempt.CapacityReservationID, &attempt.BudgetReservationID,
		&attempt.WorkloadReference, &attempt.CreatedAt, &attempt.UpdatedAt, &attempt.StartedAt, &attempt.CompletedAt)
	return attempt, err
}

var _ ports.AgentRunRepository = (*AgentRunRepository)(nil)
