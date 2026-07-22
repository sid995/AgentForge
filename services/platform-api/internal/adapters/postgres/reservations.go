package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type SchedulerReservationRepository struct{ database *database.Pool }

func NewSchedulerReservationRepository(pool *database.Pool) *SchedulerReservationRepository {
	return &SchedulerReservationRepository{database: pool}
}

func (repository *SchedulerReservationRepository) Reserve(ctx context.Context, request ports.ReservationRequest) (domain.ReservationBundle, error) {
	if err := validateReservationRequest(request); err != nil {
		return domain.ReservationBundle{}, err
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var budgetLimit int64
	if err := tx.QueryRowContext(ctx, `select daily_budget_minor_units from scheduler_tenant_policies where tenant_id=$1 for update`, request.TenantID).Scan(&budgetLimit); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	var lockedCluster string
	if err := tx.QueryRowContext(ctx, `select id from clusters where id=$1 for update`, request.ClusterID).Scan(&lockedCluster); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if existing, found, err := findActiveReservation(ctx, tx, request); err != nil {
		return domain.ReservationBundle{}, err
	} else if found {
		if existing.Capacity.ClusterID != request.ClusterID || existing.Capacity.CPUMillis != request.CPUMillis || existing.Capacity.MemoryMiB != request.MemoryMiB || existing.Budget.AmountMinorUnits != request.BudgetMinorUnits || existing.Capacity.AttemptID != request.AttemptID {
			return domain.ReservationBundle{}, domain.ErrConflict
		}
		if err := tx.Commit(); err != nil {
			return domain.ReservationBundle{}, translateError(err)
		}
		return existing, nil
	}

	var availableCPU, availableMemory int64
	if err := tx.QueryRowContext(ctx, `select allocatable_cpu_millis, allocatable_memory_mib from cluster_capacity_snapshots where cluster_id=$1 order by observed_at desc limit 1`, request.ClusterID).Scan(&availableCPU, &availableMemory); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	var reservedCPU, reservedMemory int64
	if err := tx.QueryRowContext(ctx, `select coalesce(sum(cpu_millis),0), coalesce(sum(memory_mib),0) from capacity_reservations where cluster_id=$1 and state='RESERVED' and expires_at>$2`, request.ClusterID, request.Now.UTC()).Scan(&reservedCPU, &reservedMemory); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if availableCPU-reservedCPU < request.CPUMillis || availableMemory-reservedMemory < request.MemoryMiB {
		return domain.ReservationBundle{}, domain.ErrCapacityUnavailable
	}
	usageDate := request.Now.UTC().Truncate(24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `insert into tenant_daily_budget_usage (tenant_id, usage_date, spent_minor_units, reserved_minor_units, updated_at) values ($1,$2,0,0,$3) on conflict do nothing`, request.TenantID, usageDate, request.Now.UTC()); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	var spent, reserved int64
	if err := tx.QueryRowContext(ctx, `select spent_minor_units, reserved_minor_units from tenant_daily_budget_usage where tenant_id=$1 and usage_date=$2 for update`, request.TenantID, usageDate).Scan(&spent, &reserved); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if request.BudgetMinorUnits > budgetLimit-spent-reserved {
		return domain.ReservationBundle{}, domain.ErrBudgetUnavailable
	}
	bundle, err := newReservationBundle(request, usageDate)
	if err != nil {
		return domain.ReservationBundle{}, err
	}
	if _, err := tx.ExecContext(ctx, `insert into capacity_reservations (id,tenant_id,run_id,attempt_id,attempt_number,cluster_id,cpu_millis,memory_mib,state,expires_at,created_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, bundle.Capacity.ID, bundle.Capacity.TenantID, bundle.Capacity.RunID, bundle.Capacity.AttemptID, bundle.Capacity.AttemptNumber, bundle.Capacity.ClusterID, bundle.Capacity.CPUMillis, bundle.Capacity.MemoryMiB, bundle.Capacity.State, bundle.Capacity.ExpiresAt, bundle.Capacity.CreatedAt); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if _, err := tx.ExecContext(ctx, `insert into budget_reservations (id,capacity_reservation_id,tenant_id,run_id,attempt_number,usage_date,amount_minor_units,state,expires_at,created_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, bundle.Budget.ID, bundle.Budget.CapacityReservationID, bundle.Budget.TenantID, bundle.Budget.RunID, bundle.Budget.AttemptNumber, usageDate, bundle.Budget.AmountMinorUnits, bundle.Budget.State, bundle.Budget.ExpiresAt, bundle.Budget.CreatedAt); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if _, err := tx.ExecContext(ctx, `update tenant_daily_budget_usage set reserved_minor_units=reserved_minor_units+$3, updated_at=$4 where tenant_id=$1 and usage_date=$2`, request.TenantID, usageDate, request.BudgetMinorUnits, request.Now.UTC()); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.ReservationBundle{}, translateError(err)
	}
	return bundle, nil
}

func (repository *SchedulerReservationRepository) Release(ctx context.Context, tenantID, runID uuid.UUID, attemptNumber int, now time.Time) error {
	_, err := repository.transition(ctx, tenantID, runID, attemptNumber, now, domain.ReservationReleased)
	return err
}

func (repository *SchedulerReservationRepository) Settle(ctx context.Context, tenantID, runID uuid.UUID, attemptNumber int, now time.Time) error {
	_, err := repository.transition(ctx, tenantID, runID, attemptNumber, now, domain.ReservationSettled)
	return err
}

func (repository *SchedulerReservationRepository) ReclaimExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	if now.IsZero() || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("expired reservation reclaim request is invalid")
	}
	rows, err := repository.database.Raw().QueryContext(ctx, `select tenant_id, run_id, attempt_number from capacity_reservations where state='RESERVED' and expires_at<=$1 order by expires_at,id limit $2`, now.UTC(), limit)
	if err != nil {
		return 0, translateError(err)
	}
	type expired struct {
		tenantID, runID uuid.UUID
		attempt         int
	}
	items := make([]expired, 0, limit)
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.tenantID, &item.runID, &item.attempt); err != nil {
			_ = rows.Close()
			return 0, translateError(err)
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, translateError(err)
	}
	if err := rows.Err(); err != nil {
		return 0, translateError(err)
	}
	reclaimed := 0
	for _, item := range items {
		changed, transitionErr := repository.transition(ctx, item.tenantID, item.runID, item.attempt, now, domain.ReservationExpired)
		if transitionErr != nil {
			return reclaimed, transitionErr
		}
		if changed {
			reclaimed++
		}
	}
	return reclaimed, nil
}

func (repository *SchedulerReservationRepository) transition(ctx context.Context, tenantID, runID uuid.UUID, attemptNumber int, now time.Time, target domain.ReservationState) (bool, error) {
	if tenantID == uuid.Nil || runID == uuid.Nil || attemptNumber < 1 || now.IsZero() {
		return false, fmt.Errorf("reservation transition identity is invalid")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return false, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select tenant_id from scheduler_tenant_policies where tenant_id=$1 for update`, tenantID); err != nil {
		return false, translateError(err)
	}
	var capacityID, budgetID uuid.UUID
	var state domain.ReservationState
	var usageDate time.Time
	var amount int64
	err = tx.QueryRowContext(ctx, `select capacity.id,budget.id,capacity.state,budget.usage_date,budget.amount_minor_units from capacity_reservations capacity join budget_reservations budget on budget.capacity_reservation_id=capacity.id where capacity.tenant_id=$1 and capacity.run_id=$2 and capacity.attempt_number=$3 order by capacity.created_at desc limit 1 for update of capacity,budget`, tenantID, runID, attemptNumber).Scan(&capacityID, &budgetID, &state, &usageDate, &amount)
	if err != nil {
		return false, translateError(err)
	}
	if state == target {
		if err := tx.Commit(); err != nil {
			return false, translateError(err)
		}
		return false, nil
	}
	if state != domain.ReservationReserved {
		return false, domain.ErrConflict
	}
	if target == domain.ReservationExpired {
		var expiresAt time.Time
		if err := tx.QueryRowContext(ctx, `select expires_at from capacity_reservations where id=$1`, capacityID).Scan(&expiresAt); err != nil {
			return false, translateError(err)
		}
		if expiresAt.After(now.UTC()) {
			if err := tx.Commit(); err != nil {
				return false, translateError(err)
			}
			return false, nil
		}
	}
	timestampColumn := "released_at"
	if target == domain.ReservationSettled {
		timestampColumn = "settled_at"
	}
	if _, err := tx.ExecContext(ctx, `update capacity_reservations set state=$2, `+timestampColumn+`=$3 where id=$1 and state='RESERVED'`, capacityID, target, now.UTC()); err != nil {
		return false, translateError(err)
	}
	if _, err := tx.ExecContext(ctx, `update budget_reservations set state=$2, `+timestampColumn+`=$3 where id=$1 and state='RESERVED'`, budgetID, target, now.UTC()); err != nil {
		return false, translateError(err)
	}
	if target == domain.ReservationSettled {
		_, err = tx.ExecContext(ctx, `update tenant_daily_budget_usage set reserved_minor_units=reserved_minor_units-$3, spent_minor_units=spent_minor_units+$3, updated_at=$4 where tenant_id=$1 and usage_date=$2`, tenantID, usageDate, amount, now.UTC())
	} else {
		_, err = tx.ExecContext(ctx, `update tenant_daily_budget_usage set reserved_minor_units=reserved_minor_units-$3, updated_at=$4 where tenant_id=$1 and usage_date=$2`, tenantID, usageDate, amount, now.UTC())
	}
	if err != nil {
		return false, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return false, translateError(err)
	}
	return true, nil
}

func validateReservationRequest(request ports.ReservationRequest) error {
	if request.TenantID == uuid.Nil || request.RunID == uuid.Nil || request.AttemptID == uuid.Nil || request.AttemptNumber < 1 || request.ClusterID == "" || request.CPUMillis < 1 || request.CPUMillis > 128000 || request.MemoryMiB < 1 || request.MemoryMiB > 524288 || request.BudgetMinorUnits < 0 || request.Now.IsZero() || request.TTL <= 0 || request.TTL > 24*time.Hour {
		return fmt.Errorf("reservation request is invalid")
	}
	return nil
}

func newReservationBundle(request ports.ReservationRequest, usageDate time.Time) (domain.ReservationBundle, error) {
	capacityID, err := uuid.NewV7()
	if err != nil {
		return domain.ReservationBundle{}, err
	}
	budgetID, err := uuid.NewV7()
	if err != nil {
		return domain.ReservationBundle{}, err
	}
	expires := request.Now.UTC().Add(request.TTL)
	capacity := domain.CapacityReservation{ID: capacityID, TenantID: request.TenantID, RunID: request.RunID, AttemptID: request.AttemptID, AttemptNumber: request.AttemptNumber, ClusterID: request.ClusterID, CPUMillis: request.CPUMillis, MemoryMiB: request.MemoryMiB, State: domain.ReservationReserved, ExpiresAt: expires, CreatedAt: request.Now.UTC()}
	if err := capacity.Validate(request.Now); err != nil {
		return domain.ReservationBundle{}, err
	}
	budget := domain.BudgetReservation{ID: budgetID, CapacityReservationID: capacityID, TenantID: request.TenantID, RunID: request.RunID, AttemptNumber: request.AttemptNumber, UsageDate: usageDate, AmountMinorUnits: request.BudgetMinorUnits, State: domain.ReservationReserved, ExpiresAt: expires, CreatedAt: request.Now.UTC()}
	return domain.ReservationBundle{Capacity: capacity, Budget: budget}, nil
}

func findActiveReservation(ctx context.Context, tx *sql.Tx, request ports.ReservationRequest) (domain.ReservationBundle, bool, error) {
	var bundle domain.ReservationBundle
	err := tx.QueryRowContext(ctx, `select capacity.id,capacity.tenant_id,capacity.run_id,capacity.attempt_id,capacity.attempt_number,capacity.cluster_id,capacity.cpu_millis,capacity.memory_mib,capacity.state,capacity.expires_at,capacity.created_at,budget.id,budget.capacity_reservation_id,budget.usage_date,budget.amount_minor_units,budget.state,budget.expires_at,budget.created_at from capacity_reservations capacity join budget_reservations budget on budget.capacity_reservation_id=capacity.id where capacity.tenant_id=$1 and capacity.run_id=$2 and capacity.attempt_number=$3 and capacity.state='RESERVED' for update`, request.TenantID, request.RunID, request.AttemptNumber).Scan(&bundle.Capacity.ID, &bundle.Capacity.TenantID, &bundle.Capacity.RunID, &bundle.Capacity.AttemptID, &bundle.Capacity.AttemptNumber, &bundle.Capacity.ClusterID, &bundle.Capacity.CPUMillis, &bundle.Capacity.MemoryMiB, &bundle.Capacity.State, &bundle.Capacity.ExpiresAt, &bundle.Capacity.CreatedAt, &bundle.Budget.ID, &bundle.Budget.CapacityReservationID, &bundle.Budget.UsageDate, &bundle.Budget.AmountMinorUnits, &bundle.Budget.State, &bundle.Budget.ExpiresAt, &bundle.Budget.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReservationBundle{}, false, nil
	}
	if err != nil {
		return domain.ReservationBundle{}, false, translateError(err)
	}
	bundle.Budget.TenantID, bundle.Budget.RunID, bundle.Budget.AttemptNumber = bundle.Capacity.TenantID, bundle.Capacity.RunID, bundle.Capacity.AttemptNumber
	return bundle, true, nil
}

var _ ports.SchedulerReservationRepository = (*SchedulerReservationRepository)(nil)
