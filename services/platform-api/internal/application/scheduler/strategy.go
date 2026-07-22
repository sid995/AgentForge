package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

const (
	StrategyLeastLoaded    = "least-loaded"
	StrategyRegionAffinity = "region-affinity"
)

// SelectionRequest is the bounded workload input to a cluster strategy.
type SelectionRequest struct {
	CPUMillis       int64
	MemoryMiB       int64
	PreferredRegion string
}

// SelectionDecision is a safe, durable-ready explanation of one choice.
type SelectionDecision struct {
	ClusterID               string
	Strategy                string
	Reason                  string
	Score                   int64
	ProjectedUtilizationBPS int64
	QueuedWorkloads         int
	EffectiveCost           int64
	UsedRegionFallback      bool
	DecidedAt               time.Time
}

// NoCapacityError is a temporary scheduling deferral, not a run failure.
type NoCapacityError struct{ Reason string }

func (err NoCapacityError) Error() string { return err.Reason }

// Strategy ranks already metadata-eligible clusters deterministically.
type Strategy interface {
	Name() string
	Select([]domain.ClusterCandidate, SelectionRequest, time.Time) (SelectionDecision, error)
}

type leastLoadedStrategy struct{}
type regionAffinityStrategy struct{}

func NewStrategy(name string) (Strategy, error) {
	switch name {
	case StrategyLeastLoaded:
		return leastLoadedStrategy{}, nil
	case StrategyRegionAffinity:
		return regionAffinityStrategy{}, nil
	default:
		return nil, fmt.Errorf("unsupported scheduler strategy %q", name)
	}
}

func (leastLoadedStrategy) Name() string { return StrategyLeastLoaded }

func (leastLoadedStrategy) Select(candidates []domain.ClusterCandidate, request SelectionRequest, now time.Time) (SelectionDecision, error) {
	return rankCandidates(candidates, request, now, StrategyLeastLoaded, false)
}

func (regionAffinityStrategy) Name() string { return StrategyRegionAffinity }

func (strategy regionAffinityStrategy) Select(candidates []domain.ClusterCandidate, request SelectionRequest, now time.Time) (SelectionDecision, error) {
	if request.PreferredRegion != "" {
		regional := make([]domain.ClusterCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Cluster.Region == request.PreferredRegion {
				regional = append(regional, candidate)
			}
		}
		if decision, err := rankCandidates(regional, request, now, StrategyRegionAffinity, false); err == nil {
			decision.Reason = "selected least-loaded cluster in preferred region"
			return decision, nil
		}
	}
	decision, err := rankCandidates(candidates, request, now, StrategyRegionAffinity, request.PreferredRegion != "")
	if err != nil {
		return SelectionDecision{}, err
	}
	if decision.UsedRegionFallback {
		decision.Reason = "preferred region had no sufficient capacity; selected global least-loaded cluster"
	}
	return decision, nil
}

type scoredCandidate struct {
	clusterID string
	score     int64
	projected int64
	queued    int
	cost      int64
}

func rankCandidates(candidates []domain.ClusterCandidate, request SelectionRequest, now time.Time, strategy string, fallback bool) (SelectionDecision, error) {
	if request.CPUMillis <= 0 || request.MemoryMiB <= 0 || now.IsZero() {
		return SelectionDecision{}, fmt.Errorf("selection request is invalid")
	}
	ranked := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		capacity := candidate.Capacity
		cluster := candidate.Cluster
		if capacity.AllocatableCPUMillis < request.CPUMillis || capacity.AllocatableMemoryMiB < request.MemoryMiB || cluster.Status != domain.ClusterActive || cluster.Maintenance {
			continue
		}
		cpuBPS := ceilingRatio(request.CPUMillis, capacity.AllocatableCPUMillis, 10_000)
		memoryBPS := ceilingRatio(request.MemoryMiB, capacity.AllocatableMemoryMiB, 10_000)
		projected := max(cpuBPS, memoryBPS)
		cost := int64(cluster.CPUCostMinorUnits) + int64(cluster.MemoryGiBCostMinorUnits)
		score := projected*1000/int64(cluster.SchedulingWeight) + int64(capacity.QueuedWorkloads)*100 + cost
		ranked = append(ranked, scoredCandidate{clusterID: cluster.ID, score: score, projected: projected, queued: capacity.QueuedWorkloads, cost: cost})
	}
	if len(ranked) == 0 {
		return SelectionDecision{}, NoCapacityError{Reason: "no eligible cluster has sufficient capacity"}
	}
	sort.Slice(ranked, func(left, right int) bool {
		a, b := ranked[left], ranked[right]
		if a.score != b.score {
			return a.score < b.score
		}
		if a.projected != b.projected {
			return a.projected < b.projected
		}
		if a.queued != b.queued {
			return a.queued < b.queued
		}
		if a.cost != b.cost {
			return a.cost < b.cost
		}
		return a.clusterID < b.clusterID
	})
	selected := ranked[0]
	return SelectionDecision{
		ClusterID: selected.clusterID, Strategy: strategy,
		Reason: "selected lowest deterministic capacity score", Score: selected.score,
		ProjectedUtilizationBPS: selected.projected, QueuedWorkloads: selected.queued,
		EffectiveCost: selected.cost, UsedRegionFallback: fallback, DecidedAt: now.UTC(),
	}, nil
}

func ceilingRatio(numerator, denominator, scale int64) int64 {
	quotient, remainder := numerator/denominator, numerator%denominator
	result := quotient * scale
	if remainder != 0 {
		result += (remainder*scale-1)/denominator + 1
	}
	return result
}

// SelectionObserver avoids high-cardinality tenant/run metric labels.
type SelectionObserver interface{ ObserveSelection(strategy, result string) }

// Selector provides the observable application boundary around a Strategy.
type Selector struct {
	strategy Strategy
	logger   *slog.Logger
	observer SelectionObserver
}

func NewSelector(strategy Strategy, logger *slog.Logger, observer SelectionObserver) *Selector {
	return &Selector{strategy: strategy, logger: logger, observer: observer}
}

func (selector *Selector) Select(ctx context.Context, candidates []domain.ClusterCandidate, request SelectionRequest, now time.Time) (SelectionDecision, error) {
	decision, err := selector.strategy.Select(candidates, request, now)
	result := "selected"
	if err != nil {
		result = "deferred"
	}
	if selector.observer != nil {
		selector.observer.ObserveSelection(selector.strategy.Name(), result)
	}
	if selector.logger != nil {
		attributes := []any{"strategy", selector.strategy.Name(), "result", result}
		if err == nil {
			attributes = append(attributes, "cluster_id", decision.ClusterID, "reason", decision.Reason)
		}
		selector.logger.InfoContext(ctx, "scheduler cluster selection", attributes...)
	}
	return decision, err
}
