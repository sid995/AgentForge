//go:build integration

package runs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestServiceCreatesAndReplaysTenantScopedRun(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredIntegrationEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredIntegrationEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	if err := migrations.Apply(migrationURL, integrationMigrationDirectory(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	adminPool := openIntegrationPool(t, migrationURL)
	defer adminPool.Close()
	appPool := openIntegrationPool(t, appURL)
	defer appPool.Close()

	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	tenant, err := domain.NewTenant("api-"+uuid.NewString()[:8], "API tenant", now)
	if err != nil {
		t.Fatalf("new tenant: %v", err)
	}
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	project, err := domain.NewProject(tenant.ID, "API Project", "https://example.test/repository.git", now)
	if err != nil {
		t.Fatalf("new project: %v", err)
	}
	projects := postgresadapter.NewProjectRepository(appPool)
	if err := projects.Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	service := NewService(postgresadapter.NewAgentRunRepository(appPool), projects, func() time.Time { return now })
	caller := identity.Identity{TenantID: tenant.ID, Subject: "developer@example.test", Role: identity.RoleDeveloper}
	input := CreateInput{PromptReference: "vault://prompts/" + uuid.NewString(), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: "api-" + uuid.NewString()}
	created, replayed, err := service.Create(ctx, caller, project.ID, input)
	if err != nil || replayed || created.Status != domain.AgentRunQueued || created.CreatedBy != caller.Subject {
		t.Fatalf("create run=%#v replayed=%t error=%v", created, replayed, err)
	}
	if countOutboxEvents(t, adminPool, created.ID) != 1 {
		t.Fatal("create did not transactionally write its requested outbox event")
	}
	replayedRun, replayed, err := service.Create(ctx, caller, project.ID, input)
	if err != nil || !replayed || replayedRun.ID != created.ID {
		t.Fatalf("replay run=%#v replayed=%t error=%v", replayedRun, replayed, err)
	}
	if countOutboxEvents(t, adminPool, created.ID) != 1 {
		t.Fatal("idempotent replay created another outbox event")
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status = 'SCHEDULING', scheduler_lease_owner = 'integration-test', scheduler_lease_expires_at = $2, version = version + 1 where id = $1`, created.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("advance run after initial response: %v", err)
	}
	replayedRun, replayed, err = service.Create(ctx, caller, project.ID, input)
	if err != nil || !replayed || replayedRun.Status != domain.AgentRunQueued || replayedRun.UpdatedAt != created.UpdatedAt {
		t.Fatalf("replay did not return original response run=%#v replayed=%t error=%v", replayedRun, replayed, err)
	}
	if got, err := service.Get(ctx, caller, created.ID); err != nil || got.ID != created.ID {
		t.Fatalf("get run=%#v error=%v", got, err)
	}
	if listed, err := service.List(ctx, caller, project.ID, ports.AgentRunPage{Limit: 10}); err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list runs=%#v error=%v", listed, err)
	}
}

func TestServiceCancellationAndRetryAreTransactionalAndIdempotent(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredIntegrationEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredIntegrationEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	if err := migrations.Apply(migrationURL, integrationMigrationDirectory(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	adminPool := openIntegrationPool(t, migrationURL)
	defer adminPool.Close()
	appPool := openIntegrationPool(t, appURL)
	defer appPool.Close()

	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	tenant, err := domain.NewTenant("commands-"+uuid.NewString()[:8], "Commands tenant", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	project, err := domain.NewProject(tenant.ID, "Commands Project", "https://example.test/commands.git", now)
	if err != nil {
		t.Fatal(err)
	}
	projects := postgresadapter.NewProjectRepository(appPool)
	if err := projects.Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	service := NewService(postgresadapter.NewAgentRunRepository(appPool), projects, func() time.Time { return now })
	caller := identity.Identity{TenantID: tenant.ID, Subject: "developer@example.test", Role: identity.RoleDeveloper}

	cancelled, _, err := service.Create(ctx, caller, project.ID, CreateInput{PromptReference: "vault://prompts/" + uuid.NewString(), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: "cancel-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("create cancellation run: %v", err)
	}
	cancelResult, err := service.Cancel(ctx, caller, cancelled.ID, "cancel-command", "requested by test")
	if err != nil || cancelResult.Replayed || cancelResult.Run.Status != domain.AgentRunCancelling || cancelResult.Run.CancellationRequestedBy != caller.Subject {
		t.Fatalf("cancel result=%#v error=%v", cancelResult, err)
	}
	if got := outboxEventTypesForRun(t, adminPool, cancelled.ID); len(got) != 2 || got[1] != "agent-run.cancel-requested.v1" {
		t.Fatalf("cancellation outbox=%v", got)
	}
	replayedCancel, err := service.Cancel(ctx, caller, cancelled.ID, "cancel-command", "requested by test")
	if err != nil || !replayedCancel.Replayed || replayedCancel.Run.Status != domain.AgentRunCancelling {
		t.Fatalf("cancel replay=%#v error=%v", replayedCancel, err)
	}
	if got := outboxEventTypesForRun(t, adminPool, cancelled.ID); len(got) != 2 {
		t.Fatalf("cancel replay inserted events=%v", got)
	}

	retryable, _, err := service.Create(ctx, caller, project.ID, CreateInput{PromptReference: "vault://prompts/" + uuid.NewString(), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: "retry-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("create retry run: %v", err)
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `update agent_runs set status='EXECUTION_FAILED', failure_category='TRANSIENT_DEPENDENCY', completed_at=$1, updated_at=$1, version=version+1 where id=$2`, now.Add(time.Minute), retryable.ID); err != nil {
		t.Fatalf("make run retryable: %v", err)
	}
	retryResult, err := service.Retry(ctx, caller, retryable.ID, "retry-command")
	if err != nil || retryResult.Replayed || retryResult.Run.Status != domain.AgentRunQueued || retryResult.Run.AttemptCount != 2 {
		t.Fatalf("retry result=%#v error=%v", retryResult, err)
	}
	if got := outboxEventTypesForRun(t, adminPool, retryable.ID); len(got) != 2 || got[1] != "agent-run.retry-requested.v1" {
		t.Fatalf("retry outbox=%v", got)
	}
	var attempts int
	if err := adminPool.Raw().QueryRowContext(ctx, `select count(*) from agent_run_attempts where run_id=$1`, retryable.ID).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("retry attempt count=%d error=%v", attempts, err)
	}
	replayedRetry, err := service.Retry(ctx, caller, retryable.ID, "retry-command")
	if err != nil || !replayedRetry.Replayed || replayedRetry.Run.AttemptCount != 2 {
		t.Fatalf("retry replay=%#v error=%v", replayedRetry, err)
	}
	if got := outboxEventTypesForRun(t, adminPool, retryable.ID); len(got) != 2 {
		t.Fatalf("retry replay inserted events=%v", got)
	}
	if _, err := service.Retry(ctx, caller, retryable.ID, "different-command"); !errors.Is(err, domain.ErrRetryNotAllowed) {
		t.Fatalf("retry from queued error=%v, want ErrRetryNotAllowed", err)
	}
}

func countOutboxEvents(t *testing.T, pool *database.Pool, runID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.Raw().QueryRowContext(context.Background(), `select count(*) from outbox_events where run_id = $1`, runID).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	return count
}

func outboxEventTypesForRun(t *testing.T, pool *database.Pool, runID uuid.UUID) []string {
	t.Helper()
	rows, err := pool.Raw().QueryContext(context.Background(), `select event_type from outbox_events where run_id=$1 order by created_at, event_id`, runID)
	if err != nil {
		t.Fatalf("list outbox event types: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			t.Fatalf("scan outbox event type: %v", err)
		}
		types = append(types, eventType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox event types: %v", err)
	}
	return types
}

func openIntegrationPool(t *testing.T, url string) *database.Pool {
	t.Helper()
	pool, err := database.Open(context.Background(), config.DatabaseConfig{URL: url, MaxConns: 4, MaxIdleConns: 1, MaxConnLifetime: time.Minute, AcquireTimeout: time.Second, ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("open database pool: %v", err)
	}
	return pool
}

func integrationMigrationDirectory(t *testing.T) string {
	t.Helper()
	directory, err := filepath.Abs("../../../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migration directory: %v", err)
	}
	return directory
}

func requiredIntegrationEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not configured", name)
	}
	return value
}
