// Package postgres contains domain-specific PostgreSQL repository adapters.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// TenantRepository is the privileged PostgreSQL adapter for platform tenant provisioning.
type TenantRepository struct {
	database *database.Pool
}

// ProjectRepository is the tenant-scoped PostgreSQL adapter for projects.
type ProjectRepository struct {
	database *database.Pool
}

// NewTenantRepository constructs the tenant provisioning adapter.
func NewTenantRepository(pool *database.Pool) *TenantRepository {
	return &TenantRepository{database: pool}
}

// NewProjectRepository constructs the tenant-scoped project adapter.
func NewProjectRepository(pool *database.Pool) *ProjectRepository {
	return &ProjectRepository{database: pool}
}

// Create persists a platform-provisioned tenant. This operation requires a
// privileged provisioning connection and is deliberately not exposed by HTTP.
func (repository *TenantRepository) Create(ctx context.Context, tenant domain.Tenant) error {
	_, err := repository.database.Raw().ExecContext(ctx, `
		insert into tenants (id, slug, name, created_at, updated_at)
		values ($1, $2, $3, $4, $5)`,
		tenant.ID, tenant.Slug, tenant.Name, tenant.CreatedAt, tenant.UpdatedAt)
	return translateError(err)
}

// Create persists a project inside one tenant-scoped transaction.
func (repository *ProjectRepository) Create(ctx context.Context, project domain.Project) error {
	tx, err := repository.database.BeginTenant(ctx, project.TenantID)
	if err != nil {
		return translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		insert into projects (id, tenant_id, name, repository_url, version, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7)`,
		project.ID, project.TenantID, project.Name, project.RepositoryURL, project.Version, project.CreatedAt, project.UpdatedAt)
	if err != nil {
		return translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return translateError(err)
	}
	return nil
}

// Get returns a project only when it belongs to the supplied tenant.
func (repository *ProjectRepository) Get(ctx context.Context, tenantID, projectID uuid.UUID) (domain.Project, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return domain.Project{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	project, err := scanProject(tx.QueryRowContext(ctx, `
		select id, tenant_id, name, repository_url, version, created_at, updated_at
		from projects where tenant_id = $1 and id = $2`, tenantID, projectID))
	if err != nil {
		return domain.Project{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Project{}, translateError(err)
	}
	return project, nil
}

// List returns a tenant-scoped, keyset-paginated project history.
func (repository *ProjectRepository) List(ctx context.Context, tenantID uuid.UUID, page ports.ProjectPage) ([]domain.Project, error) {
	if (page.AfterCreatedAt == nil) != (page.AfterID == nil) {
		return nil, fmt.Errorf("project page cursor must include creation time and ID")
	}
	if page.Limit <= 0 || page.Limit > 100 {
		page.Limit = 50
	}
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		select id, tenant_id, name, repository_url, version, created_at, updated_at
		from projects
		where tenant_id = $1
		  and ($2::timestamptz is null or (created_at, id) < ($2, $3))
		order by created_at desc, id desc
		limit $4`, tenantID, page.AfterCreatedAt, page.AfterID, page.Limit)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()
	projects := make([]domain.Project, 0, page.Limit)
	for rows.Next() {
		project, scanErr := scanProject(rows)
		if scanErr != nil {
			return nil, translateError(scanErr)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, translateError(err)
	}
	return projects, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanProject(row rowScanner) (domain.Project, error) {
	var project domain.Project
	err := row.Scan(&project.ID, &project.TenantID, &project.Name, &project.RepositoryURL, &project.Version, &project.CreatedAt, &project.UpdatedAt)
	return project, err
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return domain.ErrConflict
		case "23503":
			return domain.ErrNotFound
		case "42501":
			return domain.ErrForbidden
		}
	}
	return fmt.Errorf("PostgreSQL operation failed: %w", err)
}

var _ ports.TenantRepository = (*TenantRepository)(nil)
var _ ports.ProjectRepository = (*ProjectRepository)(nil)
