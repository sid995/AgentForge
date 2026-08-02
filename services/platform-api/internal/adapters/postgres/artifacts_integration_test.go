//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

func TestArtifactsAreTenantScopedAndImmutable(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	adminPool := openPool(t, migrationURL, 2)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 2)
	defer appPool.Close()

	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, uniqueSlug("artifact-alpha"), now)
	tenantB := mustTenant(t, uniqueSlug("artifact-bravo"), now)
	tenants := postgresadapter.NewTenantRepository(adminPool)
	if err := tenants.Create(ctx, tenantA); err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	if err := tenants.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	project := mustProject(t, tenantA.ID, "Artifact Project "+uuid.NewString()[:8], now)
	if err := postgresadapter.NewProjectRepository(appPool).Create(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	run, attempt := mustRunAndAttempt(t, tenantA.ID, project.ID, "artifact-"+uuid.NewString(), now)
	if err := postgresadapter.NewAgentRunRepository(appPool).Create(ctx, run, attempt); err != nil {
		t.Fatalf("create run and attempt: %v", err)
	}

	artifact := mustArtifact(t, tenantA.ID, project.ID, run.ID, attempt, now)
	repository := postgresadapter.NewArtifactRepository(appPool)
	if err := repository.Create(ctx, artifact); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	got, err := repository.Get(ctx, tenantA.ID, artifact.ID)
	if err != nil || got != artifact {
		t.Fatalf("get artifact = %#v, error = %v", got, err)
	}
	artifacts, err := repository.ListByRun(ctx, tenantA.ID, run.ID)
	if err != nil || len(artifacts) != 1 || artifacts[0] != artifact {
		t.Fatalf("list artifacts = %#v, error = %v", artifacts, err)
	}
	if _, err := repository.Get(ctx, tenantB.ID, artifact.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant get error = %v, want ErrNotFound", err)
	}
	artifacts, err = repository.ListByRun(ctx, tenantB.ID, run.ID)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("cross-tenant list = %#v, error = %v", artifacts, err)
	}

	assertArtifactRLSAndImmutability(t, appPool, tenantA.ID, tenantB.ID, artifact)
}

func mustArtifact(t *testing.T, tenantID, projectID, runID uuid.UUID, attempt domain.AgentRunAttempt, now time.Time) domain.Artifact {
	t.Helper()
	artifact, err := domain.NewArtifact(domain.NewArtifactInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		RunID:          runID,
		AttemptID:      attempt.ID,
		AttemptNumber:  attempt.AttemptNumber,
		ObjectKey:      "tenants/" + tenantID.String() + "/projects/" + projectID.String() + "/runs/" + runID.String() + "/attempts/1/logs/runner.jsonl",
		ContentType:    "application/x-ndjson",
		SizeBytes:      512,
		SHA256:         "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RetentionClass: domain.ArtifactRetentionHot,
		CreatedBy:      "agent-runner",
	}, now)
	if err != nil {
		t.Fatalf("new artifact: %v", err)
	}
	return artifact
}

func assertArtifactRLSAndImmutability(t *testing.T, pool interface{ Raw() *sql.DB }, tenantA, tenantB uuid.UUID, artifact domain.Artifact) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Raw().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin artifact RLS transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "select set_config('app.tenant_id', $1, true)", tenantB.String()); err != nil {
		t.Fatalf("set tenant B context: %v", err)
	}
	var found uuid.UUID
	err = tx.QueryRowContext(ctx, "select id from artifacts where id = $1", artifact.ID).Scan(&found)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RLS direct artifact lookup error = %v, want no rows", err)
	}
	_, err = tx.ExecContext(ctx, `
		insert into artifacts (id, tenant_id, project_id, run_id, attempt_id, attempt_number, object_key, content_type, size_bytes, sha256, retention_class, created_by, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		uuid.Must(uuid.NewV7()), artifact.TenantID, artifact.ProjectID, artifact.RunID, artifact.AttemptID, artifact.AttemptNumber,
		artifact.ObjectKey+".blocked", artifact.ContentType, artifact.SizeBytes, artifact.SHA256, artifact.RetentionClass, artifact.CreatedBy, artifact.CreatedAt)
	if err == nil {
		t.Fatal("RLS allowed a cross-tenant artifact write")
	}

	_, err = pool.Raw().ExecContext(ctx, "update artifacts set content_type = 'text/plain' where id = $1", artifact.ID)
	if err == nil {
		t.Fatal("artifact update succeeded despite restrictive application-role grants")
	}
}
