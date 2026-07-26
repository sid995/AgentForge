package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type engineQueue struct {
	mu  sync.Mutex
	run ports.ClaimedRun
}

func (queue *engineQueue) Claim(context.Context, ports.SchedulerClaimRequest) ([]ports.ClaimedRun, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.run.ID == uuid.Nil {
		return nil, nil
	}
	run := queue.run
	queue.run = ports.ClaimedRun{}
	return []ports.ClaimedRun{run}, nil
}
func (*engineQueue) Renew(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, version int64, _ time.Time, _ time.Duration) (int64, error) {
	return version + 1, nil
}

type engineEligibility struct{ decision domain.EligibilityDecision }

func (value engineEligibility) Evaluate(context.Context, ports.ClaimedRun, time.Time) (domain.EligibilityDecision, error) {
	return value.decision, nil
}

type engineRegistry struct{ candidates []domain.ClusterCandidate }

func (value engineRegistry) ListCandidates(context.Context, uuid.UUID, string, string, time.Time, time.Duration) ([]domain.ClusterCandidate, error) {
	return value.candidates, nil
}

type engineSelector struct {
	decision SelectionDecision
	err      error
}

func (value engineSelector) Select(context.Context, []domain.ClusterCandidate, SelectionRequest, time.Time) (SelectionDecision, error) {
	return value.decision, value.err
}

type engineIntent struct {
	scheduled chan ports.SchedulingIntentRequest
	deferred  chan ports.CapacityWaitRequest
	rejected  chan ports.SchedulingRejectRequest
}

func (value *engineIntent) Schedule(_ context.Context, request ports.SchedulingIntentRequest) (ports.SchedulingIntentResult, error) {
	value.scheduled <- request
	return ports.SchedulingIntentResult{}, nil
}
func (value *engineIntent) DeferCapacity(_ context.Context, request ports.CapacityWaitRequest) (ports.SchedulingIntentResult, error) {
	value.deferred <- request
	return ports.SchedulingIntentResult{}, nil
}
func (value *engineIntent) Reject(_ context.Context, request ports.SchedulingRejectRequest) (ports.SchedulingIntentResult, error) {
	value.rejected <- request
	return ports.SchedulingIntentResult{}, nil
}

func TestEnginePollsAndSchedulesWithBoundedWorkers(t *testing.T) {
	run := engineClaim()
	queue := &engineQueue{run: run}
	intent := &engineIntent{scheduled: make(chan ports.SchedulingIntentRequest, 1), deferred: make(chan ports.CapacityWaitRequest, 1), rejected: make(chan ports.SchedulingRejectRequest, 1)}
	cluster := domain.ExecutionCluster{ID: "cluster-engine", Status: domain.ClusterActive, SchedulingWeight: 100, CPUCostMinorUnits: 2, MemoryGiBCostMinorUnits: 3}
	configuration := EngineConfig{Owner: "engine-test", PollInterval: 10 * time.Millisecond, LeaseDuration: time.Second, BatchSize: 1, WorkerCount: 1, WorkQueueSize: 1, AgingInterval: time.Minute, ClusterFreshness: time.Minute, ReservationTTL: time.Minute, DeferralDuration: time.Second, ExecutionIntent: engineExecutionIntent()}
	engine, err := NewEngine(configuration, NewClaimer(queue, nil, nil), engineEligibility{domain.EligibilityDecision{Outcome: domain.EligibilityEligible}}, engineRegistry{[]domain.ClusterCandidate{{Cluster: cluster}}}, engineSelector{decision: SelectionDecision{ClusterID: cluster.ID, Strategy: StrategyLeastLoaded, Score: 10}}, intent, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()
	select {
	case request := <-intent.scheduled:
		if request.AttemptID != run.AttemptID || request.BudgetMinorUnits != 5 || request.Claim.Version != run.Version+1 {
			t.Fatalf("request=%#v", request)
		}
		cancel()
	case <-time.After(time.Second):
		t.Fatal("scheduler did not process claimed run")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEngineFinalizesDeferralAndRejection(t *testing.T) {
	for _, test := range []struct {
		name     string
		decision domain.EligibilityDecision
		want     string
	}{{"defer", domain.EligibilityDecision{Outcome: domain.EligibilityDefer, Code: "DAILY_BUDGET_LIMIT", Explanation: "budget exhausted", NextEligibleAt: timePointer(time.Now().Add(time.Minute))}, "defer"}, {"reject", domain.EligibilityDecision{Outcome: domain.EligibilityReject, Code: "TENANT_SUSPENDED", Explanation: "tenant suspended"}, "reject"}} {
		t.Run(test.name, func(t *testing.T) {
			intent := &engineIntent{scheduled: make(chan ports.SchedulingIntentRequest, 1), deferred: make(chan ports.CapacityWaitRequest, 1), rejected: make(chan ports.SchedulingRejectRequest, 1)}
			configuration := EngineConfig{Owner: "engine-test", PollInterval: time.Second, LeaseDuration: 2 * time.Second, BatchSize: 1, WorkerCount: 1, WorkQueueSize: 1, AgingInterval: time.Minute, ClusterFreshness: time.Minute, ReservationTTL: time.Minute, DeferralDuration: time.Second}
			engine, err := NewEngine(configuration, NewClaimer(&engineQueue{}, nil, nil), engineEligibility{test.decision}, engineRegistry{}, engineSelector{}, intent, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			engine.process(context.Background(), engineClaim())
			if test.want == "defer" {
				select {
				case <-intent.deferred:
				default:
					t.Fatal("deferral not finalized")
				}
			} else {
				select {
				case <-intent.rejected:
				default:
					t.Fatal("rejection not finalized")
				}
			}
		})
	}
}

func engineClaim() ports.ClaimedRun {
	return ports.ClaimedRun{ID: uuid.Must(uuid.NewV7()), TenantID: uuid.Must(uuid.NewV7()), ProjectID: uuid.Must(uuid.NewV7()), Runtime: "python-3.12", PromptReference: "vault://tasks/engine", MaxAttempts: 2, ExecutionProfile: "standard", CreatedBy: "actor", CPUMillis: 500, MemoryMiB: 1024, TimeoutSeconds: 300, AttemptNumber: 1, AttemptID: uuid.Must(uuid.NewV7()), AttemptVersion: 1, Version: 2, SchedulerLeaseOwner: "engine-test", SchedulerLeaseExpiry: time.Now().Add(time.Minute)}
}
func timePointer(value time.Time) *time.Time { return &value }
func engineExecutionIntent() config.ExecutionIntentConfig {
	return config.ExecutionIntentConfig{RunnerImages: map[string]string{"python-3.12": "registry.example.test/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, WorkspaceSizeGiB: 10, ArtifactDestinationRef: "artifact-store"}
}
