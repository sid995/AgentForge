// Package scheduler coordinates bounded, observable queue scheduling work.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type ClaimMetrics interface {
	Claimed(string, int)
	QueueAge(time.Duration)
	LeaseRenewed(string)
}

type noClaimMetrics struct{}

func (noClaimMetrics) Claimed(string, int)    {}
func (noClaimMetrics) QueueAge(time.Duration) {}
func (noClaimMetrics) LeaseRenewed(string)    {}

type Claimer struct {
	repository ports.SchedulerQueueRepository
	metrics    ClaimMetrics
	logger     *slog.Logger
}

func NewClaimer(repository ports.SchedulerQueueRepository, metrics ClaimMetrics, logger *slog.Logger) *Claimer {
	if metrics == nil {
		metrics = noClaimMetrics{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Claimer{repository: repository, metrics: metrics, logger: logger}
}

func (claimer *Claimer) Claim(ctx context.Context, request ports.SchedulerClaimRequest) ([]ports.ClaimedRun, error) {
	claimed, err := claimer.repository.Claim(ctx, request)
	if err != nil {
		claimer.metrics.Claimed("error", 0)
		claimer.logger.ErrorContext(ctx, "scheduler queue claim failed", slog.String("owner", request.Owner), slog.String("error", "database claim failed"))
		return nil, err
	}
	oldest := time.Duration(0)
	for _, run := range claimed {
		age := request.Now.Sub(run.CreatedAt)
		if age > oldest {
			oldest = age
		}
	}
	result := "empty"
	if len(claimed) > 0 {
		result = "claimed"
		claimer.metrics.QueueAge(oldest)
	}
	claimer.metrics.Claimed(result, len(claimed))
	claimer.logger.InfoContext(ctx, "scheduler queue claim completed", slog.String("owner", request.Owner), slog.String("result", result), slog.Int("count", len(claimed)), slog.Int64("oldest_queue_age_ms", oldest.Milliseconds()))
	return claimed, nil
}

func (claimer *Claimer) Renew(ctx context.Context, tenantID, runID uuid.UUID, owner string, expectedVersion int64, now time.Time, lease time.Duration) (int64, error) {
	version, err := claimer.repository.Renew(ctx, tenantID, runID, owner, expectedVersion, now, lease)
	if err != nil {
		result := "error"
		if errors.Is(err, domain.ErrVersionConflict) {
			result = "conflict"
		}
		claimer.metrics.LeaseRenewed(result)
		return 0, err
	}
	claimer.metrics.LeaseRenewed("renewed")
	return version, nil
}
