//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestSchedulingIntentAtomicallyAssignsReservesAndEmits(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	schedulerURL := requiredEnvironment(t, "AGENTFORGE_TEST_SCHEDULER_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 8)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 4)
	defer appPool.Close()
	schedulerPool := openPool(t, schedulerURL, 8)
	defer schedulerPool.Close()
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status='PROVISIONING',scheduler_lease_owner=null,scheduler_lease_expires_at=null where status in ('QUEUED','SCHEDULING','CAPACITY_WAIT')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC)
	tenant := mustTenant(t, uniqueSlug("intent"), now)
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	project := mustProject(t, tenant.ID, "Intent "+uuid.NewString()[:8], now)
	if err := postgresadapter.NewProjectRepository(appPool).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `insert into scheduler_tenant_policies (tenant_id,max_concurrent_runs,max_user_concurrent_runs,max_queued_runs,max_cpu_millis,max_memory_mib,daily_budget_minor_units,allowed_runtimes,allowed_execution_profiles,deferral_seconds,updated_at) values ($1,10,10,20,10000,20000,1000,array['python-3.12'],array['standard'],30,$2)`, tenant.ID, now); err != nil {
		t.Fatal(err)
	}
	cluster, err := domain.NewExecutionCluster(domain.ExecutionCluster{ID: "cluster-intent", Region: "us-east-1", Status: domain.ClusterActive, SupportedRuntimes: []string{"python-3.12"}, SupportedExecutionProfiles: []string{"standard"}, SchedulingWeight: 100, LastHeartbeatAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresadapter.NewClusterRegistry(schedulerPool).Register(ctx, cluster, domain.ClusterCapacity{ClusterID: cluster.ID, ObservedAt: now, AllocatableCPUMillis: 4000, AllocatableMemoryMiB: 8192}, nil); err != nil {
		t.Fatal(err)
	}
	run, attempt := mustRunAndAttempt(t, tenant.ID, project.ID, "intent-"+uuid.NewString(), now)
	if err := postgresadapter.NewAgentRunRepository(appPool).Create(ctx, run, attempt); err != nil {
		t.Fatal(err)
	}
	queue := postgresadapter.NewSchedulerQueueRepository(schedulerPool)
	claimed, err := queue.Claim(ctx, ports.SchedulerClaimRequest{Owner: "scheduler-intent", Now: now.Add(time.Second), LeaseDuration: time.Minute, BatchSize: 1, AgingInterval: time.Minute, MaximumAgingBoost: 10})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	desired := domain.AgentRunDesiredState{RunnerImage: "registry.example.test/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Runtime: run.Runtime, ExecutionProfile: "standard", TaskReference: run.PromptReference, TimeoutSeconds: run.TimeoutSeconds, MaxAttempts: run.MaxAttempts, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300, RetryableFailureCategories: []domain.FailureCategory{domain.FailureTransientDependency}, CPUMillis: run.CPUMillis, MemoryMiB: run.MemoryMiB, CPULimitMillis: run.CPUMillis, MemoryLimitMiB: run.MemoryMiB, WorkspaceSizeGiB: 10, WorkspaceRetentionPolicy: "Delete", NetworkProfile: "Isolated", ArtifactDestinationRef: "artifact-store"}
	request := ports.SchedulingIntentRequest{Claim: claimed[0], LeaseOwner: "scheduler-intent", AttemptID: attempt.ID, AttemptVersion: attempt.Version, ClusterID: cluster.ID, ClusterFreshness: 5 * time.Minute, Strategy: "least-loaded", SelectionScore: 2500, BudgetMinorUnits: 100, ReservationTTL: 5 * time.Minute, CorrelationID: run.ID.String(), CausationID: "claim-intent", Now: now.Add(2 * time.Second), DesiredState: desired}
	repository := postgresadapter.NewSchedulingIntentRepository(schedulerPool)
	mismatched := request
	mismatched.DesiredState.TaskReference = "vault://tasks/other"
	if _, err := repository.Schedule(ctx, mismatched); err == nil {
		t.Fatal("mismatched desired state was accepted")
	}
	invalid := request
	invalid.LeaseOwner = "wrong-owner"
	if _, err := repository.Schedule(ctx, invalid); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("wrong-owner error=%v", err)
	}
	var beforeReservations int
	if err := adminPool.Raw().QueryRowContext(ctx, `select count(*) from capacity_reservations where run_id=$1`, run.ID).Scan(&beforeReservations); err != nil || beforeReservations != 0 {
		t.Fatalf("failed transaction reservations=%d err=%v", beforeReservations, err)
	}
	type result struct {
		value ports.SchedulingIntentResult
		err   error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, scheduleErr := repository.Schedule(ctx, request)
			results <- result{value, scheduleErr}
		}()
	}
	wait.Wait()
	close(results)
	var first ports.SchedulingIntentResult
	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("schedule error=%v", outcome.err)
		}
		if first.EventID == uuid.Nil {
			first = outcome.value
		} else if outcome.value.EventID != first.EventID || outcome.value.CapacityReservationID != first.CapacityReservationID {
			t.Fatalf("non-idempotent results %#v %#v", first, outcome.value)
		}
	}
	changed := request
	changed.ClusterID = "cluster-other"
	if _, err := repository.Schedule(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed replay error=%v", err)
	}
	changed = request
	changed.DesiredState.WorkspaceSizeGiB++
	if _, err := repository.Schedule(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed desired-state replay error=%v", err)
	}
	var runStatus, attemptStatus, eventType string
	var capacityCount, budgetCount, handoffCount int
	if err := adminPool.Raw().QueryRowContext(ctx, `select run.status,attempt.status,event.event_type,(select count(*) from capacity_reservations where run_id=run.id),(select count(*) from budget_reservations where run_id=run.id),(select count(*) from agentrun_handoff_intents where run_id=run.id) from agent_runs run join agent_run_attempts attempt on attempt.run_id=run.id join outbox_events event on event.run_id=run.id and event.event_type=$2 where run.id=$1`, run.ID, events.AgentRunScheduledType).Scan(&runStatus, &attemptStatus, &eventType, &capacityCount, &budgetCount, &handoffCount); err != nil {
		t.Fatal(err)
	}
	if runStatus != "PROVISIONING" || attemptStatus != "STARTING" || eventType != events.AgentRunScheduledType || capacityCount != 1 || budgetCount != 1 || handoffCount != 1 {
		t.Fatalf("state run=%s attempt=%s event=%s capacity=%d budget=%d handoff=%d", runStatus, attemptStatus, eventType, capacityCount, budgetCount, handoffCount)
	}

	waitRun, waitAttempt := mustRunAndAttempt(t, tenant.ID, project.ID, "wait-"+uuid.NewString(), now.Add(3*time.Second))
	if err := postgresadapter.NewAgentRunRepository(appPool).Create(ctx, waitRun, waitAttempt); err != nil {
		t.Fatal(err)
	}
	claimed, err = queue.Claim(ctx, ports.SchedulerClaimRequest{Owner: "scheduler-wait", Now: now.Add(4 * time.Second), LeaseDuration: time.Minute, BatchSize: 1, AgingInterval: time.Minute, MaximumAgingBoost: 10})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("wait claim=%#v err=%v", claimed, err)
	}
	deferRequest := ports.CapacityWaitRequest{Claim: claimed[0], LeaseOwner: "scheduler-wait", ReasonCode: "NO_CAPACITY", Reason: "no eligible cluster has sufficient capacity", NextEligibleAt: now.Add(time.Minute), CorrelationID: waitRun.ID.String(), CausationID: "claim-wait", Now: now.Add(5 * time.Second)}
	deferred, err := repository.DeferCapacity(ctx, deferRequest)
	if err != nil || deferred.EventID == uuid.Nil {
		t.Fatalf("deferred=%#v err=%v", deferred, err)
	}
	replayed, err := repository.DeferCapacity(ctx, deferRequest)
	if err != nil || !replayed.Replayed || replayed.RunVersion != deferred.RunVersion {
		t.Fatalf("replayed=%#v err=%v", replayed, err)
	}
	if err := adminPool.Raw().QueryRowContext(ctx, `select status from agent_runs where id=$1`, waitRun.ID).Scan(&runStatus); err != nil || runStatus != "CAPACITY_WAIT" {
		t.Fatalf("wait status=%s err=%v", runStatus, err)
	}
	if err := adminPool.Raw().QueryRowContext(ctx, `select event_type from outbox_events where run_id=$1 and event_type=$2`, waitRun.ID, events.AgentRunCapacityWaitType).Scan(&eventType); err != nil {
		t.Fatal(err)
	}

	rejectRun, rejectAttempt := mustRunAndAttempt(t, tenant.ID, project.ID, "reject-"+uuid.NewString(), now.Add(6*time.Second))
	if err := postgresadapter.NewAgentRunRepository(appPool).Create(ctx, rejectRun, rejectAttempt); err != nil {
		t.Fatal(err)
	}
	claimed, err = queue.Claim(ctx, ports.SchedulerClaimRequest{Owner: "scheduler-reject", Now: now.Add(7 * time.Second), LeaseDuration: time.Minute, BatchSize: 1, AgingInterval: time.Minute, MaximumAgingBoost: 10})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reject claim=%#v err=%v", claimed, err)
	}
	rejected, err := repository.Reject(ctx, ports.SchedulingRejectRequest{Claim: claimed[0], LeaseOwner: "scheduler-reject", ReasonCode: "TENANT_SUSPENDED", Reason: "tenant is suspended", CorrelationID: rejectRun.ID.String(), CausationID: "claim-reject", Now: now.Add(8 * time.Second)})
	if err != nil || rejected.EventID == uuid.Nil {
		t.Fatalf("rejected=%#v err=%v", rejected, err)
	}
	if err := adminPool.Raw().QueryRowContext(ctx, `select status,event.event_type from agent_runs run join outbox_events event on event.run_id=run.id and event.event_type=$2 where run.id=$1`, rejectRun.ID, events.AgentRunFailedType).Scan(&runStatus, &eventType); err != nil || runStatus != "POLICY_REJECTED" {
		t.Fatalf("reject state=%s event=%s err=%v", runStatus, eventType, err)
	}
	var attemptVersion int64
	if err := adminPool.Raw().QueryRowContext(ctx, `select status,version from agent_run_attempts where id=$1`, rejectAttempt.ID).Scan(&attemptStatus, &attemptVersion); err != nil || attemptStatus != "CANCELLED" || attemptVersion != rejected.AttemptVersion {
		t.Fatalf("reject attempt status=%s version=%d result=%#v err=%v", attemptStatus, attemptVersion, rejected, err)
	}
	replayed, err = repository.Reject(ctx, ports.SchedulingRejectRequest{Claim: claimed[0], LeaseOwner: "scheduler-reject", ReasonCode: "TENANT_SUSPENDED", Reason: "tenant is suspended", CorrelationID: rejectRun.ID.String(), CausationID: "claim-reject", Now: now.Add(8 * time.Second)})
	if err != nil || !replayed.Replayed || replayed.EventID != rejected.EventID || replayed.RunVersion != rejected.RunVersion || replayed.AttemptVersion != rejected.AttemptVersion {
		t.Fatalf("replayed rejection=%#v err=%v", replayed, err)
	}
}
