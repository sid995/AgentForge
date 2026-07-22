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
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestReservationsAreAtomicIdempotentAndCapacityBounded(t *testing.T) {
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
	now := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	tenant := mustTenant(t, uniqueSlug("reserve"), now)
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	project := mustProject(t, tenant.ID, "Reservations "+uuid.NewString()[:8], now)
	if err := postgresadapter.NewProjectRepository(appPool).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `insert into scheduler_tenant_policies (tenant_id,max_concurrent_runs,max_user_concurrent_runs,max_queued_runs,max_cpu_millis,max_memory_mib,daily_budget_minor_units,allowed_runtimes,allowed_execution_profiles,deferral_seconds,updated_at) values ($1,10,10,20,10000,20000,1000,array['python-3.12'],array['standard'],30,$2)`, tenant.ID, now); err != nil {
		t.Fatal(err)
	}
	cluster, err := domain.NewExecutionCluster(domain.ExecutionCluster{ID: "cluster-reserve", Region: "us-east-1", Status: domain.ClusterActive, SupportedRuntimes: []string{"python-3.12"}, SupportedExecutionProfiles: []string{"standard"}, SchedulingWeight: 100, LastHeartbeatAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresadapter.NewClusterRegistry(schedulerPool).Register(ctx, cluster, domain.ClusterCapacity{ClusterID: cluster.ID, ObservedAt: now, AllocatableCPUMillis: 1000, AllocatableMemoryMiB: 2048}, nil); err != nil {
		t.Fatal(err)
	}
	runRepository := postgresadapter.NewAgentRunRepository(appPool)
	makeRequest := func(suffix string) ports.ReservationRequest {
		run, attempt := mustRunAndAttempt(t, tenant.ID, project.ID, "reserve-"+suffix+uuid.NewString(), now)
		if err := runRepository.Create(ctx, run, attempt); err != nil {
			t.Fatal(err)
		}
		return ports.ReservationRequest{TenantID: tenant.ID, RunID: run.ID, AttemptID: attempt.ID, AttemptNumber: 1, ClusterID: cluster.ID, CPUMillis: 700, MemoryMiB: 1024, BudgetMinorUnits: 200, Now: now, TTL: time.Minute}
	}
	requests := []ports.ReservationRequest{makeRequest("a"), makeRequest("b")}
	repository := postgresadapter.NewSchedulerReservationRepository(schedulerPool)
	type result struct {
		bundle domain.ReservationBundle
		err    error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, request := range requests {
		wait.Add(1)
		go func(request ports.ReservationRequest) {
			defer wait.Done()
			bundle, reserveErr := repository.Reserve(ctx, request)
			results <- result{bundle, reserveErr}
		}(request)
	}
	wait.Wait()
	close(results)
	var winner ports.ReservationRequest
	var reservation domain.ReservationBundle
	successes, deferred := 0, 0
	for index := 0; index < 2; index++ {
		outcome := <-results
		if outcome.err == nil {
			successes++
			reservation = outcome.bundle
			for _, request := range requests {
				if request.RunID == reservation.Capacity.RunID {
					winner = request
				}
			}
		} else if errors.Is(outcome.err, domain.ErrCapacityUnavailable) {
			deferred++
		} else {
			t.Fatalf("reserve error=%v", outcome.err)
		}
	}
	if successes != 1 || deferred != 1 {
		t.Fatalf("successes=%d deferred=%d", successes, deferred)
	}
	replay, err := repository.Reserve(ctx, winner)
	if err != nil || replay.Capacity.ID != reservation.Capacity.ID || replay.Budget.ID != reservation.Budget.ID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	changed := winner
	changed.MemoryMiB++
	if _, err := repository.Reserve(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed replay error=%v", err)
	}
	if err := repository.Release(ctx, winner.TenantID, winner.RunID, winner.AttemptNumber, now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repository.Release(ctx, winner.TenantID, winner.RunID, winner.AttemptNumber, now.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}

	settleRequest := makeRequest("settle")
	settleRequest.CPUMillis = 500
	bundle, err := repository.Reserve(ctx, settleRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Settle(ctx, settleRequest.TenantID, settleRequest.RunID, 1, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repository.Settle(ctx, settleRequest.TenantID, settleRequest.RunID, 1, now.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	var spent, reserved int64
	if err := adminPool.Raw().QueryRowContext(ctx, `select spent_minor_units,reserved_minor_units from tenant_daily_budget_usage where tenant_id=$1 and usage_date=$2`, tenant.ID, now.Format("2006-01-02")).Scan(&spent, &reserved); err != nil || spent != bundle.Budget.AmountMinorUnits || reserved != 0 {
		t.Fatalf("usage spent=%d reserved=%d err=%v", spent, reserved, err)
	}

	expiredRequest := makeRequest("expired")
	expiredRequest.CPUMillis = 400
	expiredRequest.Now = now.Add(30 * time.Second)
	expiredRequest.TTL = time.Second
	if _, err := repository.Reserve(ctx, expiredRequest); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := repository.ReclaimExpired(ctx, now.Add(32*time.Second), 10)
	if err != nil || reclaimed != 1 {
		t.Fatalf("reclaimed=%d err=%v", reclaimed, err)
	}
	reclaimed, err = repository.ReclaimExpired(ctx, now.Add(33*time.Second), 10)
	if err != nil || reclaimed != 0 {
		t.Fatalf("second reclaimed=%d err=%v", reclaimed, err)
	}
}
