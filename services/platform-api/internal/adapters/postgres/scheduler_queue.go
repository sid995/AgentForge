package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// SchedulerQueueRepository performs short, cross-tenant claim transactions
// through the isolated scheduler database role.
type SchedulerQueueRepository struct{ database *database.Pool }

func NewSchedulerQueueRepository(pool *database.Pool) *SchedulerQueueRepository {
	return &SchedulerQueueRepository{database: pool}
}

func (repository *SchedulerQueueRepository) Claim(ctx context.Context, request ports.SchedulerClaimRequest) ([]ports.ClaimedRun, error) {
	request.Owner = strings.TrimSpace(request.Owner)
	if request.Owner == "" || len(request.Owner) > 160 || request.Now.IsZero() || request.LeaseDuration <= 0 || request.BatchSize < 1 || request.BatchSize > 1000 || request.AgingInterval <= 0 || request.MaximumAgingBoost < 0 || request.MaximumAgingBoost > 1000 {
		return nil, fmt.Errorf("scheduler claim requires bounded owner, clock, lease, batch, and aging policy")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin scheduler claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := request.Now.UTC()
	rows, err := tx.QueryContext(ctx, `
		with base as materialized (
			select run.id, run.tenant_id, run.version, run.created_at,
				run.priority::integer + least($6::integer, floor(greatest(0, extract(epoch from ($1::timestamptz - run.created_at))) / $5::double precision)::integer) as effective_priority
			from agent_runs run
			where (
				(run.status = 'QUEUED')
				or (run.status = 'CAPACITY_WAIT' and run.scheduler_next_eligible_at <= $1)
				or (run.status = 'SCHEDULING' and run.scheduler_lease_expires_at <= $1)
			)
			and exists (
				select 1 from agent_run_attempts attempt
				where attempt.tenant_id = run.tenant_id and attempt.run_id = run.id
				  and attempt.attempt_number = run.attempt_count and attempt.status = 'PENDING'
			)
		), ranked as materialized (
			select base.*, row_number() over (
				partition by tenant_id order by effective_priority desc, created_at asc, id asc
			) as tenant_rank
			from base
		), locked as (
			select run.id, run.version, ranked.effective_priority, ranked.tenant_rank, ranked.created_at
			from agent_runs run join ranked on ranked.id = run.id and ranked.version = run.version
			order by ranked.tenant_rank asc, ranked.effective_priority desc, ranked.created_at asc, run.tenant_id asc, run.id asc
			for update of run skip locked
			limit $4
		), updated as (
			update agent_runs run
			set status = 'SCHEDULING', scheduler_lease_owner = $2,
				scheduler_lease_expires_at = $3, updated_at = $1, version = run.version + 1
			from locked
			where run.id = locked.id and run.version = locked.version
			returning run.id, run.tenant_id, run.project_id, run.runtime,
				run.execution_profile, coalesce(run.preferred_region, '') as preferred_region, run.created_by,
				run.cpu_millis, run.memory_mib, run.timeout_seconds, run.attempt_count,
				run.priority::integer, locked.effective_priority, run.version, run.created_at,
				run.scheduler_lease_owner, run.scheduler_lease_expires_at, locked.tenant_rank
		)
		select updated.id, updated.tenant_id, updated.project_id, updated.runtime, updated.execution_profile, updated.preferred_region,
			updated.created_by, updated.cpu_millis, updated.memory_mib, updated.timeout_seconds, updated.attempt_count, updated.priority,
			updated.effective_priority, updated.version, updated.created_at, updated.scheduler_lease_owner,
			updated.scheduler_lease_expires_at, attempt.id, attempt.version
		from updated join agent_run_attempts attempt
		  on attempt.tenant_id=updated.tenant_id and attempt.run_id=updated.id
		 and attempt.attempt_number=updated.attempt_count
		order by updated.tenant_rank asc, updated.effective_priority desc, updated.created_at asc, updated.tenant_id asc, updated.id asc`,
		now, request.Owner, now.Add(request.LeaseDuration), request.BatchSize, request.AgingInterval.Seconds(), request.MaximumAgingBoost)
	if err != nil {
		return nil, fmt.Errorf("claim scheduler queue: %w", err)
	}
	defer func() { _ = rows.Close() }()
	claimed := make([]ports.ClaimedRun, 0, request.BatchSize)
	for rows.Next() {
		var run ports.ClaimedRun
		if err := rows.Scan(&run.ID, &run.TenantID, &run.ProjectID, &run.Runtime, &run.ExecutionProfile, &run.PreferredRegion, &run.CreatedBy, &run.CPUMillis, &run.MemoryMiB, &run.TimeoutSeconds, &run.AttemptNumber, &run.Priority, &run.EffectivePriority, &run.Version, &run.CreatedAt, &run.SchedulerLeaseOwner, &run.SchedulerLeaseExpiry, &run.AttemptID, &run.AttemptVersion); err != nil {
			return nil, fmt.Errorf("scan scheduler claim: %w", err)
		}
		claimed = append(claimed, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scheduler claims: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit scheduler claims: %w", err)
	}
	return claimed, nil
}

func (repository *SchedulerQueueRepository) Renew(ctx context.Context, tenantID, runID uuid.UUID, owner string, expectedVersion int64, now time.Time, lease time.Duration) (int64, error) {
	owner = strings.TrimSpace(owner)
	if tenantID.Version() != 7 || runID.Version() != 7 || owner == "" || len(owner) > 160 || expectedVersion < 1 || now.IsZero() || lease <= 0 {
		return 0, fmt.Errorf("scheduler lease renewal metadata is invalid")
	}
	var version int64
	err := repository.database.Raw().QueryRowContext(ctx, `
		update agent_runs
		set scheduler_lease_expires_at = $1, updated_at = $2, version = version + 1
		where tenant_id = $3 and id = $4 and status = 'SCHEDULING'
		  and scheduler_lease_owner = $5 and scheduler_lease_expires_at > $2
		  and version = $6
		returning version`, now.UTC().Add(lease), now.UTC(), tenantID, runID, owner, expectedVersion).Scan(&version)
	if err == nil {
		return version, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("renew scheduler lease: %w", err)
	}
	var exists bool
	if checkErr := repository.database.Raw().QueryRowContext(ctx, `select exists(select 1 from agent_runs where tenant_id = $1 and id = $2)`, tenantID, runID).Scan(&exists); checkErr != nil {
		return 0, fmt.Errorf("classify scheduler lease conflict: %w", checkErr)
	}
	if !exists {
		return 0, domain.ErrNotFound
	}
	return 0, domain.ErrVersionConflict
}

var _ ports.SchedulerQueueRepository = (*SchedulerQueueRepository)(nil)
