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
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestSchedulerQueueOrderingLeasesConcurrencyAndRoleIsolation(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	schedulerURL := requiredEnvironment(t, "AGENTFORGE_TEST_SCHEDULER_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 6)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 6)
	defer appPool.Close()
	schedulerPool := openPool(t, schedulerURL, 6)
	defer schedulerPool.Close()

	// Isolate this global queue test from queued fixtures created by an earlier
	// integration test in the same fresh database.
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status = 'PROVISIONING', scheduler_lease_owner = null, scheduler_lease_expires_at = null where status in ('QUEUED', 'CAPACITY_WAIT', 'SCHEDULING')`); err != nil {
		t.Fatalf("drain pre-existing queue: %v", err)
	}

	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, uniqueSlug("scheduler-a"), now)
	tenantB := mustTenant(t, uniqueSlug("scheduler-b"), now)
	tenants := postgresadapter.NewTenantRepository(adminPool)
	if err := tenants.Create(ctx, tenantA); err != nil {
		t.Fatal(err)
	}
	if err := tenants.Create(ctx, tenantB); err != nil {
		t.Fatal(err)
	}
	projects := postgresadapter.NewProjectRepository(appPool)
	projectA := mustProject(t, tenantA.ID, "Scheduler A "+uuid.NewString()[:8], now)
	projectB := mustProject(t, tenantB.ID, "Scheduler B "+uuid.NewString()[:8], now)
	if err := projects.Create(ctx, projectA); err != nil {
		t.Fatal(err)
	}
	if err := projects.Create(ctx, projectB); err != nil {
		t.Fatal(err)
	}
	runs := postgresadapter.NewAgentRunRepository(appPool)
	oldHigh := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now.Add(-4*time.Minute))
	newHigh := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now.Add(-3*time.Minute))
	low := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now.Add(-5*time.Minute))
	otherTenant := createSchedulerRun(t, runs, tenantB.ID, projectB.ID, now.Add(-2*time.Minute))
	setPriorities(t, adminPool, map[uuid.UUID]int{oldHigh.ID: 10, newHigh.ID: 10, low.ID: 1, otherTenant.ID: 5})

	queue := postgresadapter.NewSchedulerQueueRepository(schedulerPool)
	request := ports.SchedulerClaimRequest{Owner: "scheduler-order", Now: now, LeaseDuration: 30 * time.Second, BatchSize: 4, AgingInterval: time.Hour, MaximumAgingBoost: 10}
	ordered, err := queue.Claim(ctx, request)
	if err != nil {
		t.Fatalf("ordered claim: %v", err)
	}
	wantOrder := []uuid.UUID{oldHigh.ID, otherTenant.ID, newHigh.ID, low.ID}
	if len(ordered) != len(wantOrder) {
		t.Fatalf("ordered claim count=%d", len(ordered))
	}
	for index, want := range wantOrder {
		if ordered[index].ID != want || ordered[index].TenantID == uuid.Nil || ordered[index].SchedulerLeaseOwner != request.Owner {
			t.Fatalf("ordered[%d]=%#v want run=%s", index, ordered[index], want)
		}
	}

	target := createSchedulerRun(t, runs, tenantA.ID, projectA.ID, now)
	setPriorities(t, adminPool, map[uuid.UUID]int{target.ID: 100})
	concurrentRequest := request
	concurrentRequest.BatchSize = 1
	var wait sync.WaitGroup
	results := make(chan []ports.ClaimedRun, 2)
	errorsFound := make(chan error, 2)
	for _, owner := range []string{"scheduler-one", "scheduler-two"} {
		wait.Add(1)
		go func(owner string) {
			defer wait.Done()
			candidate := concurrentRequest
			candidate.Owner = owner
			claimed, claimErr := queue.Claim(ctx, candidate)
			results <- claimed
			errorsFound <- claimErr
		}(owner)
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for claimErr := range errorsFound {
		if claimErr != nil {
			t.Fatalf("concurrent claim: %v", claimErr)
		}
	}
	claimedCount := 0
	var winning ports.ClaimedRun
	for result := range results {
		claimedCount += len(result)
		if len(result) == 1 {
			winning = result[0]
		}
	}
	if claimedCount != 1 || winning.ID != target.ID {
		t.Fatalf("concurrent claimed=%d winning=%#v", claimedCount, winning)
	}

	steal := concurrentRequest
	steal.Owner = "scheduler-steal"
	if claimed, err := queue.Claim(ctx, steal); err != nil || len(claimed) != 0 {
		t.Fatalf("active lease stolen: claimed=%#v err=%v", claimed, err)
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set scheduler_lease_expires_at = $1 where id = $2`, now.Add(-time.Second), target.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := queue.Claim(ctx, steal)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != target.ID || reclaimed[0].SchedulerLeaseOwner != steal.Owner {
		t.Fatalf("expired reclaim=%#v err=%v", reclaimed, err)
	}
	if _, err := queue.Renew(ctx, tenantB.ID, target.ID, steal.Owner, reclaimed[0].Version, now.Add(time.Second), time.Minute); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant renew error=%v", err)
	}
	renewedVersion, err := queue.Renew(ctx, target.TenantID, target.ID, steal.Owner, reclaimed[0].Version, now.Add(time.Second), time.Minute)
	if err != nil || renewedVersion != reclaimed[0].Version+1 {
		t.Fatalf("renewed version=%d err=%v", renewedVersion, err)
	}
	if _, err := queue.Renew(ctx, target.TenantID, target.ID, steal.Owner, reclaimed[0].Version, now.Add(2*time.Second), time.Minute); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale renew error=%v", err)
	}
	if claimed, err := queue.Claim(ctx, steal); err != nil || len(claimed) != 0 {
		t.Fatalf("empty queue claimed=%#v err=%v", claimed, err)
	}

	var prompt string
	if err := schedulerPool.Raw().QueryRowContext(ctx, `select prompt_reference from agent_runs where id = $1`, target.ID).Scan(&prompt); err == nil {
		t.Fatal("scheduler role could read prompt reference")
	}

	closedPool := openPool(t, schedulerURL, 1)
	closedQueue := postgresadapter.NewSchedulerQueueRepository(closedPool)
	closedPool.Close()
	if _, err := closedQueue.Claim(ctx, request); err == nil {
		t.Fatal("closed database pool did not return a transient claim error")
	}
}

func createSchedulerRun(t *testing.T, repository *postgresadapter.AgentRunRepository, tenantID, projectID uuid.UUID, createdAt time.Time) domain.AgentRun {
	t.Helper()
	run, attempt := mustRunAndAttempt(t, tenantID, projectID, "scheduler-"+uuid.NewString(), createdAt)
	if err := repository.Create(context.Background(), run, attempt); err != nil {
		t.Fatalf("create scheduler run: %v", err)
	}
	return run
}

func setPriorities(t *testing.T, pool *database.Pool, priorities map[uuid.UUID]int) {
	t.Helper()
	for runID, priority := range priorities {
		if _, err := pool.Raw().ExecContext(context.Background(), `update agent_runs set priority = $1 where id = $2`, priority, runID); err != nil {
			t.Fatalf("set scheduler priority: %v", err)
		}
	}
}
