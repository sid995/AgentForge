package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type EligibilityMetrics interface {
	Decided(string, string)
}

type noEligibilityMetrics struct{}

func (noEligibilityMetrics) Decided(string, string) {}

type EligibilityService struct {
	repository ports.SchedulerEligibilityRepository
	metrics    EligibilityMetrics
	logger     *slog.Logger
}

func NewEligibilityService(repository ports.SchedulerEligibilityRepository, metrics EligibilityMetrics, logger *slog.Logger) *EligibilityService {
	if metrics == nil {
		metrics = noEligibilityMetrics{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &EligibilityService{repository: repository, metrics: metrics, logger: logger}
}

func (service *EligibilityService) Evaluate(ctx context.Context, run ports.ClaimedRun, now time.Time) (domain.EligibilityDecision, error) {
	decision, err := service.repository.Evaluate(ctx, run, now)
	if err != nil {
		service.metrics.Decided("ERROR", "DATABASE")
		service.logger.ErrorContext(ctx, "scheduler eligibility evaluation failed", slog.String("tenant_id", run.TenantID.String()), slog.String("run_id", run.ID.String()), slog.String("error", "eligibility persistence failed"))
		return domain.EligibilityDecision{}, err
	}
	service.metrics.Decided(string(decision.Outcome), decision.Code)
	service.logger.InfoContext(ctx, "scheduler eligibility evaluated", slog.String("tenant_id", run.TenantID.String()), slog.String("run_id", run.ID.String()), slog.String("outcome", string(decision.Outcome)), slog.String("reason_code", decision.Code))
	return decision, nil
}
