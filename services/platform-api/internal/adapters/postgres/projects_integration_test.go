//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/httpapi"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestMigrationsAreRepeatableAndProjectsAreTenantIsolated(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	directory := migrationDirectory(t)

	if err := migrations.Apply(migrationURL, directory); err != nil {
		t.Fatalf("first migration application: %v", err)
	}
	if err := migrations.Apply(migrationURL, directory); err != nil {
		t.Fatalf("repeated migration application: %v", err)
	}
	version, dirty, err := migrations.Version(migrationURL, directory)
	if err != nil || dirty || version != 13 {
		t.Fatalf("migration version = %d, dirty = %t, error = %v", version, dirty, err)
	}

	adminPool := openPool(t, migrationURL, 2)
	defer adminPool.Close()
	appPool := openPool(t, appURL, 1)
	defer appPool.Close()

	adminTenants := postgresadapter.NewTenantRepository(adminPool)
	appProjects := postgresadapter.NewProjectRepository(appPool)
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, "tenant-alpha", now)
	tenantB := mustTenant(t, "tenant-bravo", now)
	if err := adminTenants.Create(ctx, tenantA); err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	if err := adminTenants.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	projectA := mustProject(t, tenantA.ID, "Project Alpha", now)
	if err := appProjects.Create(ctx, projectA); err != nil {
		t.Fatalf("create project A: %v", err)
	}
	duplicateProject := mustProject(t, tenantA.ID, "Project Alpha", now)
	if err := appProjects.Create(ctx, duplicateProject); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate project error = %v, want ErrConflict", err)
	}

	got, err := appProjects.Get(ctx, tenantA.ID, projectA.ID)
	if err != nil || got.ID != projectA.ID || got.TenantID != tenantA.ID {
		t.Fatalf("scoped project retrieval = %#v, error = %v", got, err)
	}
	if _, err := appProjects.Get(ctx, tenantB.ID, projectA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant direct ID lookup error = %v, want ErrNotFound", err)
	}
	projects, err := appProjects.List(ctx, tenantB.ID, ports.ProjectPage{Limit: 10})
	if err != nil || len(projects) != 0 {
		t.Fatalf("cross-tenant project list = %#v, error = %v", projects, err)
	}

	assertRLSRejectsCrossTenantReadAndWrite(t, appPool, tenantA.ID, tenantB.ID, projectA.ID, now)
	assertTenantContextDoesNotLeak(t, appPool, projectA.ID)
	assertPoolExhaustionIsBounded(t, appPool, tenantA.ID, tenantB.ID)

	var count int
	if err := adminPool.Raw().QueryRowContext(ctx, "select count(*) from projects where tenant_id = $1", tenantA.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration role project access count = %d, error = %v", count, err)
	}
}

func TestReadinessTracksPostgreSQLConnectivity(t *testing.T) {
	appURL := requiredEnvironment(t, "AGENTFORGE_TEST_APP_DATABASE_URL")
	pool := openPool(t, appURL, 1)
	api := httpapi.New(httpapi.Options{Build: buildinfo.Info{}, Readiness: pool.Ping})
	api.SetReady(true)
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ready status with live PostgreSQL = %d", response.Code)
	}

	pool.Close()
	response = httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status with closed PostgreSQL pool = %d", response.Code)
	}
}

func assertRLSRejectsCrossTenantReadAndWrite(t *testing.T, pool *database.Pool, tenantA, tenantB, projectID uuid.UUID, now time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Raw().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin RLS read transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "select set_config('app.tenant_id', $1, true)", tenantB.String()); err != nil {
		t.Fatalf("set tenant B context: %v", err)
	}
	var found uuid.UUID
	err = tx.QueryRowContext(ctx, "select id from projects where id = $1", projectID).Scan(&found)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RLS direct ID lookup error = %v, want no rows", err)
	}
	otherProject := mustProject(t, tenantA, "Blocked Cross Tenant Write", now)
	_, err = tx.ExecContext(ctx, `
		insert into projects (id, tenant_id, name, repository_url, version, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7)`,
		otherProject.ID, otherProject.TenantID, otherProject.Name, otherProject.RepositoryURL, otherProject.Version, otherProject.CreatedAt, otherProject.UpdatedAt)
	if err == nil {
		t.Fatal("RLS allowed a cross-tenant write")
	}
}

func assertTenantContextDoesNotLeak(t *testing.T, pool *database.Pool, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Raw().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin reused transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	var found uuid.UUID
	err = tx.QueryRowContext(ctx, "select id from projects where id = $1", projectID).Scan(&found)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tenant context leaked into pooled transaction: %v", err)
	}
}

func assertPoolExhaustionIsBounded(t *testing.T, pool *database.Pool, tenantA, tenantB uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	held, err := pool.BeginTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("begin held transaction: %v", err)
	}
	defer func() { _ = held.Rollback() }()
	timeoutContext, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
	defer cancel()
	if _, err := pool.BeginTenant(timeoutContext, tenantB); err == nil {
		t.Fatal("tenant transaction acquired a connection despite exhausted pool")
	}
}

func openPool(t *testing.T, url string, maxConns int32) *database.Pool {
	t.Helper()
	pool, err := database.Open(context.Background(), config.DatabaseConfig{
		URL:             url,
		MaxConns:        maxConns,
		MaxIdleConns:    1,
		MaxConnLifetime: time.Minute,
		AcquireTimeout:  100 * time.Millisecond,
		ConnectTimeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	return pool
}

func mustTenant(t *testing.T, slug string, now time.Time) domain.Tenant {
	t.Helper()
	tenant, err := domain.NewTenant(slug, slug, now)
	if err != nil {
		t.Fatalf("new tenant: %v", err)
	}
	return tenant
}

func mustProject(t *testing.T, tenantID uuid.UUID, name string, now time.Time) domain.Project {
	t.Helper()
	project, err := domain.NewProject(tenantID, name, "https://example.test/repository.git", now)
	if err != nil {
		t.Fatalf("new project: %v", err)
	}
	return project
}

func migrationDirectory(t *testing.T) string {
	t.Helper()
	directory, err := filepath.Abs("../../../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migration directory: %v", err)
	}
	return directory
}

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not configured", name)
	}
	return value
}
