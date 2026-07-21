//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestAgentRunRepositoryPersistsTenantScopedAttemptsAndRejectsLostUpdates(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	adminPool := openPool(t, migrationURL, 4)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 4)
	defer appPool.Close()

	now := time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, uniqueSlug("run-a"), now)
	tenantB := mustTenant(t, uniqueSlug("run-b"), now)
	adminTenants := postgresadapter.NewTenantRepository(adminPool)
	if err := adminTenants.Create(ctx, tenantA); err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	if err := adminTenants.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	projects := postgresadapter.NewProjectRepository(appPool)
	project := mustProject(t, tenantA.ID, "Run Project "+uuid.NewString()[:8], now)
	if err := projects.Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	run, err := domain.NewAgentRun(domain.NewAgentRunInput{TenantID: tenantA.ID, ProjectID: project.ID, PromptReference: "vault://prompts/" + uuid.NewString(), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: "run-" + uuid.NewString(), RequestHash: "sha256:" + uuid.NewString(), CreatedBy: "developer@example.test"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	attempt, err := domain.NewAgentRunAttempt(tenantA.ID, run.ID, 1, now)
	if err != nil {
		t.Fatalf("new initial attempt: %v", err)
	}
	runs := postgresadapter.NewAgentRunRepository(appPool)
	if err := runs.Create(ctx, run, attempt); err != nil {
		t.Fatalf("create run: %v", err)
	}

	got, err := runs.Get(ctx, tenantA.ID, run.ID)
	if err != nil || got.ID != run.ID || got.Status != domain.AgentRunQueued || got.PromptReference != run.PromptReference {
		t.Fatalf("get run = %#v, error = %v", got, err)
	}
	listed, err := runs.List(ctx, tenantA.ID, ports.AgentRunPage{ProjectID: &project.ID, Limit: 10})
	if err != nil || len(listed) != 1 || listed[0].ID != run.ID {
		t.Fatalf("list runs = %#v, error = %v", listed, err)
	}
	if _, err := runs.Get(ctx, tenantB.ID, run.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant run get error = %v, want ErrNotFound", err)
	}

	duplicate := run
	duplicate.ID = uuid.Must(uuid.NewV7())
	duplicateAttempt, err := domain.NewAgentRunAttempt(tenantA.ID, duplicate.ID, 1, now)
	if err != nil {
		t.Fatalf("new duplicate attempt: %v", err)
	}
	if err := runs.Create(ctx, duplicate, duplicateAttempt); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate idempotency key error = %v, want ErrConflict", err)
	}

	nextRun := got
	nextRun.AttemptCount = 2
	nextAttempt, err := domain.NewAgentRunAttempt(tenantA.ID, run.ID, 2, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("new next attempt: %v", err)
	}
	updated, err := runs.CreateNextAttempt(ctx, nextRun, nextAttempt, got.Version)
	if err != nil || updated.AttemptCount != 2 || updated.Version != got.Version+1 {
		t.Fatalf("create next attempt = %#v, error = %v", updated, err)
	}
	if gotAttempt, err := runs.GetAttempt(ctx, tenantA.ID, run.ID, 2); err != nil || gotAttempt.ID != nextAttempt.ID {
		t.Fatalf("get next attempt = %#v, error = %v", gotAttempt, err)
	}
	attempts, err := runs.ListAttempts(ctx, tenantA.ID, run.ID)
	if err != nil || len(attempts) != 2 || attempts[0].AttemptNumber != 1 || attempts[1].AttemptNumber != 2 {
		t.Fatalf("list attempts = %#v, error = %v", attempts, err)
	}

	pending := attempts[0]
	if err := pending.Transition(domain.AttemptStarting, "", now.Add(time.Minute)); err != nil {
		t.Fatalf("transition attempt: %v", err)
	}
	pending, err = runs.SaveAttempt(ctx, pending, attempts[0].Version)
	if err != nil || pending.Version != attempts[0].Version+1 {
		t.Fatalf("save attempt = %#v, error = %v", pending, err)
	}
	if _, err := runs.SaveAttempt(ctx, pending, attempts[0].Version); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale attempt save error = %v, want ErrVersionConflict", err)
	}

	assertRunRLSRejectsCrossTenantRead(t, appPool, tenantB.ID, run.ID)
	assertConcurrentCancellationUsesOptimisticLocking(t, runs, updated, now.Add(2*time.Minute))
}

func assertRunRLSRejectsCrossTenantRead(t *testing.T, pool interface{ Raw() *sql.DB }, tenantID, runID uuid.UUID) {
	t.Helper()
	tx, err := pool.Raw().BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin RLS transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(context.Background(), "select set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}
	var found uuid.UUID
	err = tx.QueryRowContext(context.Background(), "select id from agent_runs where id = $1", runID).Scan(&found)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RLS run lookup error = %v, want no rows", err)
	}
}

func assertConcurrentCancellationUsesOptimisticLocking(t *testing.T, repository ports.AgentRunRepository, run domain.AgentRun, now time.Time) {
	t.Helper()
	var group sync.WaitGroup
	errorsByCommand := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			candidate := run
			candidate.CancellationReason = "requested by concurrent test"
			candidate.CancellationRequestedBy = "developer@example.test"
			candidate.CancellationRequestedAt = &now
			if err := candidate.Transition(domain.AgentRunCancelling, "", now); err != nil {
				errorsByCommand <- err
				return
			}
			_, err := repository.Save(context.Background(), candidate, run.Version)
			errorsByCommand <- err
		}()
	}
	group.Wait()
	close(errorsByCommand)
	var succeeded, conflicted int
	for err := range errorsByCommand {
		if err == nil {
			succeeded++
		} else if errors.Is(err, domain.ErrVersionConflict) {
			conflicted++
		} else {
			t.Fatalf("concurrent cancellation error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent cancellation results: success=%d conflict=%d", succeeded, conflicted)
	}
}

func uniqueSlug(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString()[:8])
}
