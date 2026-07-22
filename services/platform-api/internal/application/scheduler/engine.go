package scheduler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type EngineConfig struct {
	Owner             string
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	BatchSize         int
	WorkerCount       int
	WorkQueueSize     int
	AgingInterval     time.Duration
	MaximumAgingBoost int
	ClusterFreshness  time.Duration
	ReservationTTL    time.Duration
	DeferralDuration  time.Duration
}

type eligibilityEvaluator interface {
	Evaluate(context.Context, ports.ClaimedRun, time.Time) (domain.EligibilityDecision, error)
}
type candidateRegistry interface {
	ListCandidates(context.Context, uuid.UUID, string, string, time.Time, time.Duration) ([]domain.ClusterCandidate, error)
}
type clusterSelector interface {
	Select(context.Context, []domain.ClusterCandidate, SelectionRequest, time.Time) (SelectionDecision, error)
}

type EngineMetrics interface {
	Processed(string)
	Backpressure()
	InFlight(int)
}
type noEngineMetrics struct{}

func (noEngineMetrics) Processed(string) {}
func (noEngineMetrics) Backpressure()    {}
func (noEngineMetrics) InFlight(int)     {}

type Engine struct {
	configuration EngineConfig
	claimer       *Claimer
	eligibility   eligibilityEvaluator
	registry      candidateRegistry
	selector      clusterSelector
	intent        ports.SchedulingIntentRepository
	metrics       EngineMetrics
	logger        *slog.Logger
	wake          chan struct{}
}

func NewEngine(configuration EngineConfig, claimer *Claimer, eligibility eligibilityEvaluator, registry candidateRegistry, selector clusterSelector, intent ports.SchedulingIntentRepository, metrics EngineMetrics, logger *slog.Logger) (*Engine, error) {
	if configuration.Owner == "" || configuration.PollInterval <= 0 || configuration.LeaseDuration <= configuration.PollInterval || configuration.BatchSize < 1 || configuration.WorkerCount < 1 || configuration.WorkQueueSize < configuration.WorkerCount || configuration.AgingInterval <= 0 || configuration.ClusterFreshness <= 0 || configuration.ReservationTTL <= 0 || configuration.DeferralDuration <= 0 || claimer == nil || eligibility == nil || registry == nil || selector == nil || intent == nil {
		return nil, fmt.Errorf("scheduler engine configuration and dependencies are required")
	}
	if metrics == nil {
		metrics = noEngineMetrics{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Engine{configuration: configuration, claimer: claimer, eligibility: eligibility, registry: registry, selector: selector, intent: intent, metrics: metrics, logger: logger, wake: make(chan struct{}, 1)}, nil
}

func (engine *Engine) Wake() {
	select {
	case engine.wake <- struct{}{}:
	default:
	}
}

func (engine *Engine) Run(ctx context.Context) error {
	jobs := make(chan ports.ClaimedRun, engine.configuration.WorkQueueSize)
	var workers sync.WaitGroup
	for range engine.configuration.WorkerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case run := <-jobs:
					engine.metrics.InFlight(1)
					engine.process(ctx, run)
					engine.metrics.InFlight(-1)
				}
			}
		}()
	}
	defer func() { workers.Wait() }()
	failures := 0
	for {
		if err := engine.scan(ctx, jobs); err != nil && ctx.Err() == nil {
			failures = min(failures+1, 5)
			engine.logger.ErrorContext(ctx, "scheduler scan failed", "error", "queue scan failed")
		} else {
			failures = 0
		}
		delay := engine.pollDelay(failures)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		case <-engine.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (engine *Engine) pollDelay(failures int) time.Duration {
	delay := engine.configuration.PollInterval * time.Duration(1<<failures)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(engine.configuration.Owner))
	jitter := int64(hash.Sum32()%21) - 10
	return delay + time.Duration(int64(delay)*jitter/100)
}

func (engine *Engine) scan(ctx context.Context, jobs chan<- ports.ClaimedRun) error {
	available := engine.configuration.WorkQueueSize - len(jobs)
	if available <= 0 {
		engine.metrics.Backpressure()
		return nil
	}
	batch := min(engine.configuration.BatchSize, available)
	now := time.Now().UTC()
	claimed, err := engine.claimer.Claim(ctx, ports.SchedulerClaimRequest{Owner: engine.configuration.Owner, Now: now, LeaseDuration: engine.configuration.LeaseDuration, BatchSize: batch, AgingInterval: engine.configuration.AgingInterval, MaximumAgingBoost: engine.configuration.MaximumAgingBoost})
	if err != nil {
		return err
	}
	for _, run := range claimed {
		select {
		case jobs <- run:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (engine *Engine) process(ctx context.Context, run ports.ClaimedRun) {
	now := time.Now().UTC()
	decision, err := engine.eligibility.Evaluate(ctx, run, now)
	if err != nil {
		engine.metrics.Processed("eligibility_error")
		return
	}
	renewedAt := time.Now().UTC()
	renewedVersion, err := engine.claimer.Renew(ctx, run.TenantID, run.ID, engine.configuration.Owner, run.Version, renewedAt, engine.configuration.LeaseDuration)
	if err != nil {
		engine.observeResult("lease_lost", err)
		return
	}
	run.Version = renewedVersion
	run.SchedulerLeaseExpiry = renewedAt.Add(engine.configuration.LeaseDuration)
	now = renewedAt
	correlation := run.ID.String()
	causation := "scheduler-claim:" + run.SchedulerLeaseOwner
	if decision.Outcome == domain.EligibilityReject {
		_, err = engine.intent.Reject(ctx, ports.SchedulingRejectRequest{Claim: run, LeaseOwner: engine.configuration.Owner, ReasonCode: decision.Code, Reason: decision.Explanation, CorrelationID: correlation, CausationID: causation, Now: now})
		engine.observeResult("rejected", err)
		return
	}
	if decision.Outcome == domain.EligibilityDefer {
		next := now.Add(engine.configuration.DeferralDuration)
		if decision.NextEligibleAt != nil {
			next = *decision.NextEligibleAt
		}
		if !next.After(now) {
			next = now.Add(engine.configuration.DeferralDuration)
		}
		_, err = engine.intent.DeferCapacity(ctx, ports.CapacityWaitRequest{Claim: run, LeaseOwner: engine.configuration.Owner, ReasonCode: decision.Code, Reason: decision.Explanation, NextEligibleAt: next, CorrelationID: correlation, CausationID: causation, Now: now})
		engine.observeResult("deferred", err)
		return
	}
	candidates, err := engine.registry.ListCandidates(ctx, run.TenantID, run.Runtime, run.ExecutionProfile, now, engine.configuration.ClusterFreshness)
	if err != nil {
		engine.observeResult("registry_error", err)
		return
	}
	selection, err := engine.selector.Select(ctx, candidates, SelectionRequest{CPUMillis: int64(run.CPUMillis), MemoryMiB: int64(run.MemoryMiB), PreferredRegion: run.PreferredRegion}, now)
	if err != nil {
		var noCapacity NoCapacityError
		if errors.As(err, &noCapacity) {
			_, err = engine.intent.DeferCapacity(ctx, ports.CapacityWaitRequest{Claim: run, LeaseOwner: engine.configuration.Owner, ReasonCode: "NO_CAPACITY", Reason: noCapacity.Reason, NextEligibleAt: now.Add(engine.configuration.DeferralDuration), CorrelationID: correlation, CausationID: causation, Now: now})
			engine.observeResult("capacity_wait", err)
			return
		}
		engine.observeResult("selection_error", err)
		return
	}
	var chosen domain.ClusterCandidate
	for _, candidate := range candidates {
		if candidate.Cluster.ID == selection.ClusterID {
			chosen = candidate
			break
		}
	}
	budget := estimateBudget(run, chosen.Cluster)
	_, err = engine.intent.Schedule(ctx, ports.SchedulingIntentRequest{Claim: run, LeaseOwner: engine.configuration.Owner, AttemptID: run.AttemptID, AttemptVersion: run.AttemptVersion, ClusterID: selection.ClusterID, ClusterFreshness: engine.configuration.ClusterFreshness, Strategy: selection.Strategy, SelectionScore: selection.Score, BudgetMinorUnits: budget, ReservationTTL: engine.configuration.ReservationTTL, CorrelationID: correlation, CausationID: causation, Now: now})
	engine.observeResult("scheduled", err)
}

func (engine *Engine) observeResult(success string, err error) {
	if err != nil {
		engine.metrics.Processed("error")
		engine.logger.Error("scheduler run processing failed", "result", success, "error", "scheduling transaction failed")
		return
	}
	engine.metrics.Processed(success)
}

func estimateBudget(run ports.ClaimedRun, cluster domain.ExecutionCluster) int64 {
	return ((int64(run.CPUMillis)+999)/1000)*int64(cluster.CPUCostMinorUnits) + ((int64(run.MemoryMiB)+1023)/1024)*int64(cluster.MemoryGiBCostMinorUnits)
}
