package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCapacityReservationValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := CapacityReservation{ID: uuid.Must(uuid.NewV7()), TenantID: uuid.Must(uuid.NewV7()), RunID: uuid.Must(uuid.NewV7()), AttemptID: uuid.Must(uuid.NewV7()), AttemptNumber: 1, ClusterID: "cluster-one", CPUMillis: 500, MemoryMiB: 1024, State: ReservationReserved, ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err := valid.Validate(now); err != nil {
		t.Fatal(err)
	}
	valid.CPUMillis = 0
	if err := valid.Validate(now); err == nil {
		t.Fatal("zero CPU reservation accepted")
	}
}
