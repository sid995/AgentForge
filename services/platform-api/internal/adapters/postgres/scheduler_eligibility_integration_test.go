//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestConcurrentEligibilityDefersSaturatedTenantWhileAnotherContinues(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	schedulerURL := requiredEnvironment(t, "AGENTFORGE_TEST_SCHEDULER_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 8)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 8)
	defer appPool.Close()
	schedulerPool := openPool(t, schedulerURL, 8)
	defer schedulerPool.Close()
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status = 'PROVISIONING', scheduler_lease_owner = null, scheduler_lease_expires_at = null where status in ('QUEUED', 'CAPACITY_WAIT', 'SCHEDULING')`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, uniqueSlug("quota-a"), now)
	tenantB := mustTenant(t, uniqueSlug("quota-b"), now)
	tenants := postgresadapter.NewTenantRepository(adminPool)
	if err := tenants.Create(ctx, tenantA); err != nil {
		t.Fatal(err)
	}
	if err := tenants.Create(ctx, tenantB); err != nil {
		t.Fatal(err)
	}
	projects := postgresadapter.NewProjectRepository(appPool)
	projectA := mustProject(t, tenantA.ID, "Quota A "+uuid.NewString()[:8], now)
	projectB := mustProject(t, tenantB.ID, "Quota B "+uuid.NewString()[:8], now)
	if err := projects.Create(ctx, projectA); err != nil {
		t.Fatal(err)
	}
	if err := projects.Create(ctx, projectB); err != nil {
		t.Fatal(err)
	}
	runs := postgresadapter.NewAgentRunRepository(appPool)
	active := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now.Add(-time.Minute))
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status = 'PROVISIONING' where id = $1`, active.ID); err != nil {
		t.Fatal(err)
	}
	candidateA1 := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now)
	candidateA2 := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now.Add(time.Second))
	candidateB := createSchedulerRun(t, runs, tenantB.ID, projectB.ID, now)
	setPriorities(t, adminPool, map[uuid.UUID]int{candidateA1.ID: 100, candidateA2.ID: 100, candidateB.ID: 100})
	for _, tenantID := range []uuid.UUID{tenantA.ID, tenantB.ID} {
		if _, err := adminPool.Raw().ExecContext(ctx, `
			insert into scheduler_tenant_policies (
				tenant_id, max_concurrent_runs, max_user_concurrent_runs,
				max_queued_runs, max_cpu_millis, max_memory_mib,
				daily_budget_minor_units, allowed_runtimes,
				allowed_execution_profiles, deferral_seconds, updated_at
			) values ($1, 1, 1, 10, 4000, 8192, 10000,
				array['python-3.12'], array['standard'], 45, $2)`, tenantID, now); err != nil {
			t.Fatal(err)
		}
	}

	queue := postgresadapter.NewSchedulerQueueRepository(schedulerPool)
	claimed, err := queue.Claim(ctx, ports.SchedulerClaimRequest{Owner: "scheduler-quota", Now: now.Add(2 * time.Second), LeaseDuration: time.Minute, BatchSize: 3, AgingInterval: time.Hour, MaximumAgingBoost: 10})
	if err != nil || len(claimed) != 3 {
		t.Fatalf("claim candidates=%#v err=%v", claimed, err)
	}
	byID := make(map[uuid.UUID]ports.ClaimedRun, len(claimed))
	for _, run := range claimed {
		byID[run.ID] = run
	}

	repository := postgresadapter.NewSchedulerEligibilityRepository(schedulerPool)
	type result struct {
		runID    uuid.UUID
		decision domain.EligibilityDecision
		err      error
	}
	results := make(chan result, 3)
	var wait sync.WaitGroup
	for _, runID := range []uuid.UUID{candidateA1.ID, candidateA2.ID, candidateB.ID} {
		wait.Add(1)
		go func(runID uuid.UUID) {
			defer wait.Done()
			decision, evaluateErr := repository.Evaluate(ctx, byID[runID], now.Add(3*time.Second))
			results <- result{runID: runID, decision: decision, err: evaluateErr}
		}(runID)
	}
	wait.Wait()
	close(results)
	for evaluation := range results {
		if evaluation.err != nil {
			t.Fatalf("evaluate %s: %v", evaluation.runID, evaluation.err)
		}
		if evaluation.runID == candidateB.ID {
			if evaluation.decision.Outcome != domain.EligibilityEligible {
				t.Fatalf("unrelated tenant decision=%#v", evaluation.decision)
			}
		} else if evaluation.decision.Outcome != domain.EligibilityDefer || evaluation.decision.Code != domain.EligibilityCodeTenantConcurrency {
			t.Fatalf("saturated tenant decision=%#v", evaluation.decision)
		}
	}

	var total, deferred, eligible int
	if err := adminPool.Raw().QueryRowContext(ctx, `
		select count(*), count(*) filter (where outcome = 'DEFER'),
			count(*) filter (where outcome = 'ELIGIBLE')
		from scheduler_eligibility_decisions
		where run_id in ($1, $2, $3)`, candidateA1.ID, candidateA2.ID, candidateB.ID).Scan(&total, &deferred, &eligible); err != nil {
		t.Fatal(err)
	}
	if total != 3 || deferred != 2 || eligible != 1 {
		t.Fatalf("persisted decisions total=%d deferred=%d eligible=%d", total, deferred, eligible)
	}
}
