package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ReservationState string

const (
	ReservationReserved ReservationState = "RESERVED"
	ReservationSettled  ReservationState = "SETTLED"
	ReservationReleased ReservationState = "RELEASED"
	ReservationExpired  ReservationState = "EXPIRED"
)

type CapacityReservation struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RunID         uuid.UUID
	AttemptID     uuid.UUID
	AttemptNumber int
	ClusterID     string
	CPUMillis     int64
	MemoryMiB     int64
	State         ReservationState
	ExpiresAt     time.Time
	CreatedAt     time.Time
	SettledAt     *time.Time
	ReleasedAt    *time.Time
}

type BudgetReservation struct {
	ID                    uuid.UUID
	CapacityReservationID uuid.UUID
	TenantID              uuid.UUID
	RunID                 uuid.UUID
	AttemptNumber         int
	UsageDate             time.Time
	AmountMinorUnits      int64
	State                 ReservationState
	ExpiresAt             time.Time
	CreatedAt             time.Time
	SettledAt             *time.Time
	ReleasedAt            *time.Time
}

type ReservationBundle struct {
	Capacity CapacityReservation
	Budget   BudgetReservation
}

func (reservation CapacityReservation) Validate(now time.Time) error {
	if reservation.ID.Version() != 7 || reservation.TenantID.Version() != 7 || reservation.RunID.Version() != 7 || reservation.AttemptID.Version() != 7 || reservation.AttemptNumber < 1 || !clusterIDPattern.MatchString(reservation.ClusterID) || reservation.CPUMillis < 1 || reservation.CPUMillis > 128000 || reservation.MemoryMiB < 1 || reservation.MemoryMiB > 524288 || reservation.State != ReservationReserved || !reservation.ExpiresAt.After(now.UTC()) || reservation.CreatedAt.IsZero() {
		return fmt.Errorf("capacity reservation is invalid")
	}
	return nil
}
