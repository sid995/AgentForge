//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestTransactionalOutboxAtomicityClaimRecoveryRetryAndCleanup(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	relayURL := requiredEnvironment(t, "AGENTFORGE_TEST_RELAY_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	adminPool := openPool(t, migrationURL, 4)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 4)
	defer appPool.Close()
	relayPool := openPool(t, relayURL, 4)
	defer relayPool.Close()

	now := time.Date(2026, 7, 21, 17, 0, 0, 0, time.UTC)
	tenant := mustTenant(t, uniqueSlug("outbox"), now)
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	project := mustProject(t, tenant.ID, "Outbox Project "+uuid.NewString()[:8], now)
	if err := postgresadapter.NewProjectRepository(appPool).Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	run, attempt := mustRunAndAttempt(t, tenant.ID, project.ID, "outbox-"+uuid.NewString(), now)
	runs := postgresadapter.NewAgentRunRepository(appPool)
	if err := runs.Create(ctx, run, attempt); err != nil {
		t.Fatalf("create run while broker absent: %v", err)
	}

	var runCount, outboxCount int
	if err := adminPool.Raw().QueryRowContext(ctx, "select count(*) from agent_runs where id = $1", run.ID).Scan(&runCount); err != nil {
		t.Fatalf("count run: %v", err)
	}
	if err := adminPool.Raw().QueryRowContext(ctx, "select count(*) from outbox_events where aggregate_id = $1", run.ID).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if runCount != 1 || outboxCount != 1 {
		t.Fatalf("atomic create counts run=%d outbox=%d", runCount, outboxCount)
	}

	outbox := postgresadapter.NewOutboxRepository(relayPool)
	first, err := outbox.Claim(ctx, "relay-a", now, 30*time.Second, 10)
	if err != nil {
		t.Fatalf("first claim=%#v err=%v", first, err)
	}
	target := claimedForRun(t, first, run.ID)
	for _, claimed := range first {
		if claimed.Envelope.AggregateID != run.ID {
			if err := outbox.MarkPublished(ctx, claimed.Envelope.EventID, "relay-a", now, ports.PublicationResult{}); err != nil {
				t.Fatalf("publish unrelated fixture: %v", err)
			}
		}
	}
	if target.Envelope.EventType != "agent-run.requested.v1" || target.PartitionKey != run.ID.String() {
		t.Fatalf("target claim=%#v", target)
	}
	competing, err := outbox.Claim(ctx, "relay-b", now, 30*time.Second, 10)
	if err != nil || len(competing) != 0 {
		t.Fatalf("competing claim=%#v err=%v", competing, err)
	}

	// Simulate broker success followed by relay death before MarkPublished: the
	// same event ID is reclaimed after lease expiry and may be published again.
	reclaimed, err := outbox.Claim(ctx, "relay-b", now.Add(31*time.Second), 30*time.Second, 10)
	if err != nil {
		t.Fatalf("reclaimed=%#v err=%v", reclaimed, err)
	}
	reclaimedTarget := claimedForRun(t, reclaimed, run.ID)
	if reclaimedTarget.Envelope.EventID != target.Envelope.EventID {
		t.Fatalf("reclaimed a different event: got %s want %s", reclaimedTarget.Envelope.EventID, target.Envelope.EventID)
	}
	if err := outbox.MarkFailed(ctx, reclaimedTarget.Envelope.EventID, "relay-b", now.Add(31*time.Second), "TRANSIENT", "broker unavailable", false); err != nil {
		t.Fatalf("mark transient failure: %v", err)
	}
	tooSoon, err := outbox.Claim(ctx, "relay-a", now.Add(31*time.Second), 30*time.Second, 10)
	if err != nil || len(tooSoon) != 0 {
		t.Fatalf("retry claimed too soon=%#v err=%v", tooSoon, err)
	}
	retry, err := outbox.Claim(ctx, "relay-a", now.Add(33*time.Second), 30*time.Second, 10)
	if err != nil || len(retry) != 1 {
		t.Fatalf("retry claim=%#v err=%v", retry, err)
	}
	if err := outbox.MarkPublished(ctx, retry[0].Envelope.EventID, "relay-a", now.Add(33*time.Second), ports.PublicationResult{Partition: 2, Offset: 42}); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	unpublishedRun, unpublishedAttempt := mustRunAndAttempt(t, tenant.ID, project.ID, "unpublished-"+uuid.NewString(), now.Add(time.Minute))
	if err := runs.Create(ctx, unpublishedRun, unpublishedAttempt); err != nil {
		t.Fatalf("create unpublished run: %v", err)
	}
	deleted, err := outbox.CleanupPublished(ctx, now.Add(8*24*time.Hour), 100)
	if err != nil || deleted < 1 {
		t.Fatalf("cleanup deleted=%d err=%v", deleted, err)
	}
	if err := adminPool.Raw().QueryRowContext(ctx, "select count(*) from outbox_events where aggregate_id = $1", unpublishedRun.ID).Scan(&outboxCount); err != nil || outboxCount != 1 {
		t.Fatalf("unpublished event after cleanup count=%d err=%v", outboxCount, err)
	}

	assertOutboxInsertFailureRollsBackBusinessRows(t, adminPool, runs, tenant.ID, project.ID, now.Add(2*time.Minute))
}

func claimedForRun(t *testing.T, claimed []ports.ClaimedOutboxEvent, runID uuid.UUID) ports.ClaimedOutboxEvent {
	t.Helper()
	for _, event := range claimed {
		if event.Envelope.AggregateID == runID {
			return event
		}
	}
	t.Fatalf("no claimed event for run %s", runID)
	return ports.ClaimedOutboxEvent{}
}

func assertOutboxInsertFailureRollsBackBusinessRows(t *testing.T, adminPool interface{ Raw() *sql.DB }, runs ports.AgentRunRepository, tenantID, projectID uuid.UUID, now time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := adminPool.Raw().ExecContext(ctx, "revoke insert on outbox_events from agentforge_app"); err != nil {
		t.Fatalf("revoke outbox insert: %v", err)
	}
	t.Cleanup(func() { _, _ = adminPool.Raw().ExecContext(ctx, "grant insert on outbox_events to agentforge_app") })
	run, attempt := mustRunAndAttempt(t, tenantID, projectID, "rollback-"+uuid.NewString(), now)
	if err := runs.Create(ctx, run, attempt); err == nil {
		t.Fatal("run create succeeded when outbox insert was denied")
	}
	if _, err := adminPool.Raw().ExecContext(ctx, "grant insert on outbox_events to agentforge_app"); err != nil {
		t.Fatalf("restore outbox insert: %v", err)
	}
	var count int
	if err := adminPool.Raw().QueryRowContext(ctx, "select count(*) from agent_runs where id = $1", run.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back run count=%d err=%v", count, err)
	}
}

func mustRunAndAttempt(t *testing.T, tenantID, projectID uuid.UUID, key string, now time.Time) (domain.AgentRun, domain.AgentRunAttempt) {
	t.Helper()
	run, err := domain.NewAgentRun(domain.NewAgentRunInput{TenantID: tenantID, ProjectID: projectID, PromptReference: "vault://prompts/" + uuid.NewString(), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, IdempotencyKey: key, RequestHash: "sha256:" + uuid.NewString(), CreatedBy: "developer@example.test"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	attempt, err := domain.NewAgentRunAttempt(tenantID, run.ID, 1, now)
	if err != nil {
		t.Fatalf("new attempt: %v", err)
	}
	return run, attempt
}
