//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	handoffapp "github.com/sid995/agentforge/services/platform-api/internal/application/handoff"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestHandoffRepositoryEnforcesClusterTenantAndDeduplicatesEvents(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	schedulerURL := requiredEnvironment(t, "AGENTFORGE_TEST_SCHEDULER_DATABASE_URL")
	handoffURL := requiredEnvironment(t, "AGENTFORGE_TEST_HANDOFF_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 3)
	defer adminPool.Close()
	schedulerPool := openPool(t, schedulerURL, 3)
	defer schedulerPool.Close()
	handoffPool := openPool(t, handoffURL, 3)
	defer handoffPool.Close()

	now := time.Now().UTC()
	tenant := mustTenant(t, uniqueSlug("handoff"), now)
	otherTenant := mustTenant(t, uniqueSlug("handoff-other"), now)
	tenantRepository := postgresadapter.NewTenantRepository(adminPool)
	if err := tenantRepository.Create(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if err := tenantRepository.Create(ctx, otherTenant); err != nil {
		t.Fatal(err)
	}
	cluster, err := domain.NewExecutionCluster(domain.ExecutionCluster{ID: "cluster-handoff", Region: "us-east-1", Status: domain.ClusterActive, SupportedRuntimes: []string{"python-3.12"}, SupportedExecutionProfiles: []string{"standard"}, TenantRestricted: true, SchedulingWeight: 100, LastHeartbeatAt: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresadapter.NewClusterRegistry(schedulerPool).Register(ctx, cluster, domain.ClusterCapacity{ClusterID: cluster.ID, ObservedAt: now, AllocatableCPUMillis: 2000, AllocatableMemoryMiB: 4096}, []uuid.UUID{tenant.ID}); err != nil {
		t.Fatal(err)
	}
	repository := postgresadapter.NewHandoffRepository(handoffPool)
	if err := repository.Authorize(ctx, cluster.ID, tenant.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Authorize(ctx, cluster.ID, otherTenant.ID); !errors.Is(err, handoffapp.ErrUnauthorizedCluster) {
		t.Fatalf("cross-tenant authorization error=%v", err)
	}

	runID, projectID, attemptID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	run := domain.AgentRun{ID: runID, TenantID: tenant.ID, ProjectID: projectID, Status: domain.AgentRunProvisioning, Version: 2, CPUMillis: 1000, MemoryMiB: 2048, UpdatedAt: now}
	attempt := domain.AgentRunAttempt{ID: attemptID, TenantID: tenant.ID, RunID: runID, AttemptNumber: 1, SelectedCluster: cluster.ID, ExecutionProfile: "standard"}
	desired := domain.AgentRunDesiredState{RunnerImage: "registry.example.test/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Runtime: "python-3.12", ExecutionProfile: "standard", TaskReference: "vault://tasks/handoff", TimeoutSeconds: 300, MaxAttempts: 2, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300, CPUMillis: 1000, MemoryMiB: 2048, CPULimitMillis: 1000, MemoryLimitMiB: 2048, WorkspaceSizeGiB: 10, WorkspaceRetentionPolicy: "Delete", NetworkProfile: "Isolated", ArtifactDestinationRef: "artifact-store"}
	event, err := events.NewAgentRunScheduled(run, attempt, uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "correlation", "causation")
	if err != nil {
		t.Fatal(err)
	}
	serializedDesired, _ := json.Marshal(desired)
	if _, err := adminPool.Raw().ExecContext(ctx, `insert into agentrun_handoff_intents (event_id,tenant_id,run_id,attempt_id,attempt_number,cluster_id,desired_state,created_at) values ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, event.Envelope.EventID, tenant.ID, runID, attemptID, 1, cluster.ID, serializedDesired, now); err != nil {
		t.Fatal(err)
	}
	received := ports.ReceivedEvent{Envelope: event.Envelope, Topic: event.Topic, Partition: 0, Offset: 1, Key: []byte(event.PartitionKey), Headers: events.HeadersForEnvelope(event.Envelope), Value: event.Serialized}
	var payload events.AgentRunScheduledPayload
	if err := json.Unmarshal(event.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if loaded, err := repository.LoadIntent(ctx, received, payload); err != nil || loaded.RunnerImage != desired.RunnerImage {
		t.Fatalf("loaded intent=%#v err=%v", loaded, err)
	}
	crossTenant := received
	crossTenant.Envelope.TenantID = otherTenant.ID
	if _, err := repository.LoadIntent(ctx, crossTenant, payload); !errors.Is(err, handoffapp.ErrIntentMissing) {
		t.Fatalf("cross-tenant intent load error=%v", err)
	}
	if _, err := handoffPool.Raw().ExecContext(ctx, `update agentrun_handoff_intents set desired_state=desired_state where event_id=$1`, event.Envelope.EventID); err == nil {
		t.Fatal("handoff role could mutate immutable intent")
	}
	var count int
	if err := handoffPool.Raw().QueryRowContext(ctx, `select count(*) from outbox_events`).Scan(&count); err == nil {
		t.Fatal("handoff role could read outbox events")
	}
	if _, err := adminPool.Raw().ExecContext(ctx, `update agentrun_handoff_intents set desired_state=desired_state || '{"unexpected":"value"}'::jsonb where event_id=$1`, event.Envelope.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.LoadIntent(ctx, received, payload); !errors.Is(err, handoffapp.ErrIntentInvalid) {
		t.Fatalf("unknown desired-state field error=%v", err)
	}
	if seen, err := repository.Seen(ctx, handoffapp.ConsumerIdentity, event.Envelope.EventID, tenant.ID); err != nil || seen {
		t.Fatalf("seen before mark=%v err=%v", seen, err)
	}
	if duplicate, err := repository.Mark(ctx, handoffapp.ConsumerIdentity, received, now); err != nil || duplicate {
		t.Fatalf("first mark duplicate=%v err=%v", duplicate, err)
	}
	if seen, err := repository.Seen(ctx, handoffapp.ConsumerIdentity, event.Envelope.EventID, tenant.ID); err != nil || !seen {
		t.Fatalf("seen after mark=%v err=%v", seen, err)
	}
}
