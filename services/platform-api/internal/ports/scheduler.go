package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SchedulerClaimRequest bounds one competing queue-claim transaction.
type SchedulerClaimRequest struct {
	Owner             string
	Now               time.Time
	LeaseDuration     time.Duration
	BatchSize         int
	AgingInterval     time.Duration
	MaximumAgingBoost int
}

// ClaimedRun contains only the allowlisted metadata required for scheduling.
type ClaimedRun struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	Runtime              string
	ExecutionProfile     string
	PreferredRegion      string
	CreatedBy            string
	CPUMillis            int
	MemoryMiB            int
	TimeoutSeconds       int
	AttemptNumber        int
	Priority             int
	EffectivePriority    int
	Version              int64
	CreatedAt            time.Time
	SchedulerLeaseOwner  string
	SchedulerLeaseExpiry time.Time
}

// SchedulerQueueRepository owns durable cross-tenant claims and renewals.
type SchedulerQueueRepository interface {
	Claim(context.Context, SchedulerClaimRequest) ([]ClaimedRun, error)
	Renew(context.Context, uuid.UUID, uuid.UUID, string, int64, time.Time, time.Duration) (int64, error)
}
