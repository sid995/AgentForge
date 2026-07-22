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
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestProcessedEventMarkerBusinessEffectDuplicateCrashAndReplicaSafety(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 4)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 4)
	defer appPool.Close()
	createConsumerEffectFixture(t, adminPool)

	now := time.Date(2026, 7, 21, 18, 0, 0, 0, time.UTC)
	tenant := mustTenant(t, uniqueSlug("consumer"), now)
	if err := postgresadapter.NewTenantRepository(adminPool).Create(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	project := mustProject(t, tenant.ID, "Consumer Project "+uuid.NewString()[:8], now)
	if err := postgresadapter.NewProjectRepository(appPool).Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	event := receivedRunEvent(t, tenant.ID, project.ID, now, 3, 42)
	consumer := postgresadapter.NewProcessedEventRepository(appPool, nil)

	results := make(chan struct {
		duplicate bool
		err       error
	}, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			duplicate, err := consumer.Process(ctx, "scheduler.v1", event, now, insertConsumerEffect("scheduler.v1", event))
			results <- struct {
				duplicate bool
				err       error
			}{duplicate, err}
		}()
	}
	wait.Wait()
	close(results)
	duplicates := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("replica processing: %v", result.err)
		}
		if result.duplicate {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatalf("duplicate results=%d", duplicates)
	}
	assertConsumerCounts(t, adminPool, "scheduler.v1", event.Envelope.EventID, 1, 1)

	// Simulate a crash after local commit and before broker acknowledgement.
	duplicate, err := consumer.Process(ctx, "scheduler.v1", event, now.Add(time.Second), func(context.Context, *database.TenantTx) error {
		t.Fatal("duplicate delivery executed a second business effect")
		return nil
	})
	if err != nil || !duplicate {
		t.Fatalf("post-crash duplicate=%t err=%v", duplicate, err)
	}
	assertConsumerCounts(t, adminPool, "scheduler.v1", event.Envelope.EventID, 1, 1)

	rollbackEvent := receivedRunEvent(t, tenant.ID, project.ID, now.Add(time.Minute), 1, 7)
	wantFailure := errors.New("transient dependency")
	if _, err := consumer.Process(ctx, "scheduler.v1", rollbackEvent, now, func(context.Context, *database.TenantTx) error { return wantFailure }); !errors.Is(err, wantFailure) {
		t.Fatalf("rollback error=%v", err)
	}
	assertConsumerCounts(t, adminPool, "scheduler.v1", rollbackEvent.Envelope.EventID, 0, 0)
	duplicate, err = consumer.Process(ctx, "scheduler.v1", rollbackEvent, now.Add(time.Second), insertConsumerEffect("scheduler.v1", rollbackEvent))
	if err != nil || duplicate {
		t.Fatalf("retry duplicate=%t err=%v", duplicate, err)
	}
	assertConsumerCounts(t, adminPool, "scheduler.v1", rollbackEvent.Envelope.EventID, 1, 1)
}

func receivedRunEvent(t *testing.T, tenantID, projectID uuid.UUID, now time.Time, partition int32, offset int64) ports.ReceivedEvent {
	t.Helper()
	run, attempt := mustRunAndAttempt(t, tenantID, projectID, "consumer-"+uuid.NewString(), now)
	outboxEvent, err := events.NewAgentRunRequested(run, attempt)
	if err != nil {
		t.Fatal(err)
	}
	return ports.ReceivedEvent{Envelope: outboxEvent.Envelope, Topic: outboxEvent.Topic, Partition: partition, Offset: offset, Key: []byte(outboxEvent.PartitionKey), Value: outboxEvent.Serialized, Headers: events.HeadersForEnvelope(outboxEvent.Envelope)}
}

func insertConsumerEffect(identity string, event ports.ReceivedEvent) func(context.Context, *database.TenantTx) error {
	return func(ctx context.Context, tx *database.TenantTx) error {
		_, err := tx.ExecContext(ctx, "insert into consumer_test_effects (effect_id, consumer_identity, event_id, tenant_id) values ($1, $2, $3, $4)", uuid.New(), identity, event.Envelope.EventID, event.Envelope.TenantID)
		return err
	}
}

func createConsumerEffectFixture(t *testing.T, pool *database.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Raw().ExecContext(ctx, `
		drop table if exists consumer_test_effects;
		create table consumer_test_effects (effect_id uuid primary key, consumer_identity text not null, event_id uuid not null, tenant_id uuid not null references tenants(id));
		grant select, insert on consumer_test_effects to agentforge_app;
		alter table consumer_test_effects enable row level security;
		alter table consumer_test_effects force row level security;
		create policy consumer_test_effects_tenant on consumer_test_effects to agentforge_app using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid) with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);`)
	if err != nil {
		t.Fatalf("create effect fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Raw().ExecContext(context.Background(), "drop table if exists consumer_test_effects")
	})
}

func assertConsumerCounts(t *testing.T, pool *database.Pool, identity string, eventID uuid.UUID, wantMarkers, wantEffects int) {
	t.Helper()
	var markers, effects int
	if err := pool.Raw().QueryRow("select count(*) from processed_events where consumer_identity = $1 and event_id = $2", identity, eventID).Scan(&markers); err != nil {
		t.Fatal(err)
	}
	if err := pool.Raw().QueryRow("select count(*) from consumer_test_effects where consumer_identity = $1 and event_id = $2", identity, eventID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if markers != wantMarkers || effects != wantEffects {
		t.Fatalf("markers=%d effects=%d want=%d/%d", markers, effects, wantMarkers, wantEffects)
	}
}
