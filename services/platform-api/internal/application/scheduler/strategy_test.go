package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

func TestLeastLoadedIsDeterministicAndDefersInsufficientCapacity(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	strategy, err := NewStrategy(StrategyLeastLoaded)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []domain.ClusterCandidate{
		candidate("cluster-z", "us-east-1", 2000, 4096, 0),
		candidate("cluster-a", "us-west-1", 2000, 4096, 0),
		candidate("cluster-small", "us-east-1", 499, 4096, 0),
	}
	decision, err := strategy.Select(candidates, SelectionRequest{CPUMillis: 500, MemoryMiB: 1024}, now)
	if err != nil || decision.ClusterID != "cluster-a" || decision.ProjectedUtilizationBPS != 2500 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	_, err = strategy.Select(candidates, SelectionRequest{CPUMillis: 3000, MemoryMiB: 8192}, now)
	var capacityError NoCapacityError
	if !errors.As(err, &capacityError) {
		t.Fatalf("error=%v", err)
	}
}

func TestRegionAffinityPrefersRegionAndFallsBack(t *testing.T) {
	now := time.Now().UTC()
	strategy, err := NewStrategy(StrategyRegionAffinity)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []domain.ClusterCandidate{
		candidate("cluster-local", "eu-west-1", 1000, 2048, 2),
		candidate("cluster-global", "us-east-1", 8000, 16384, 0),
	}
	request := SelectionRequest{CPUMillis: 500, MemoryMiB: 1024, PreferredRegion: "eu-west-1"}
	decision, err := strategy.Select(candidates, request, now)
	if err != nil || decision.ClusterID != "cluster-local" || decision.UsedRegionFallback {
		t.Fatalf("preferred=%#v err=%v", decision, err)
	}
	request.CPUMillis = 2000
	decision, err = strategy.Select(candidates, request, now)
	if err != nil || decision.ClusterID != "cluster-global" || !decision.UsedRegionFallback {
		t.Fatalf("fallback=%#v err=%v", decision, err)
	}
}

func TestSelectorObservesAndLogsSafeDecision(t *testing.T) {
	strategy, _ := NewStrategy(StrategyLeastLoaded)
	var output bytes.Buffer
	observer := &selectionObserver{}
	selector := NewSelector(strategy, slog.New(slog.NewJSONHandler(&output, nil)), observer)
	decision, err := selector.Select(context.Background(), []domain.ClusterCandidate{candidate("cluster-safe", "us-east-1", 2000, 4096, 0)}, SelectionRequest{CPUMillis: 500, MemoryMiB: 1024}, time.Now())
	if err != nil || observer.result != "selected" || !strings.Contains(output.String(), decision.ClusterID) {
		t.Fatalf("decision=%#v observer=%#v log=%s err=%v", decision, observer, output.String(), err)
	}
}

func candidate(id, region string, cpu, memory int64, queued int) domain.ClusterCandidate {
	return domain.ClusterCandidate{Cluster: domain.ExecutionCluster{ID: id, Region: region, Status: domain.ClusterActive, SchedulingWeight: 100}, Capacity: domain.ClusterCapacity{ClusterID: id, AllocatableCPUMillis: cpu, AllocatableMemoryMiB: memory, QueuedWorkloads: queued}}
}

type selectionObserver struct{ strategy, result string }

func (observer *selectionObserver) ObserveSelection(strategy, result string) {
	observer.strategy, observer.result = strategy, result
}
