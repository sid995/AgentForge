package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// ArtifactRepository is the PostgreSQL adapter for immutable artifact metadata.
type ArtifactRepository struct {
	database *database.Pool
}

func NewArtifactRepository(pool *database.Pool) *ArtifactRepository {
	return &ArtifactRepository{database: pool}
}

func (repository *ArtifactRepository) Create(ctx context.Context, artifact domain.Artifact) error {
	tx, err := repository.database.BeginTenant(ctx, artifact.TenantID)
	if err != nil {
		return translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		insert into artifacts (id, tenant_id, project_id, run_id, attempt_id, attempt_number, object_key, content_type, size_bytes, sha256, retention_class, created_by, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		artifact.ID, artifact.TenantID, artifact.ProjectID, artifact.RunID, artifact.AttemptID, artifact.AttemptNumber, artifact.ObjectKey, artifact.ContentType, artifact.SizeBytes, artifact.SHA256, artifact.RetentionClass, artifact.CreatedBy, artifact.CreatedAt)
	if err != nil {
		return translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return translateError(err)
	}
	return nil
}

func (repository *ArtifactRepository) Get(ctx context.Context, tenantID, artifactID uuid.UUID) (domain.Artifact, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return domain.Artifact{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	artifact, err := scanArtifact(tx.QueryRowContext(ctx, artifactSelect+` where tenant_id = $1 and id = $2`, tenantID, artifactID))
	if err != nil {
		return domain.Artifact{}, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Artifact{}, translateError(err)
	}
	return artifact, nil
}

func (repository *ArtifactRepository) ListByRun(ctx context.Context, tenantID, runID uuid.UUID) ([]domain.Artifact, error) {
	tx, err := repository.database.BeginTenant(ctx, tenantID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, artifactSelect+` where tenant_id = $1 and run_id = $2 order by created_at asc, id asc`, tenantID, runID)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()
	artifacts := make([]domain.Artifact, 0)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, translateError(err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, translateError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, translateError(err)
	}
	return artifacts, nil
}

const artifactSelect = `
	select id, tenant_id, project_id, run_id, attempt_id, attempt_number, object_key, content_type, size_bytes, sha256, retention_class, created_by, created_at
	from artifacts`

type artifactScanner interface {
	Scan(...any) error
}

func scanArtifact(row artifactScanner) (domain.Artifact, error) {
	var artifact domain.Artifact
	err := row.Scan(&artifact.ID, &artifact.TenantID, &artifact.ProjectID, &artifact.RunID, &artifact.AttemptID, &artifact.AttemptNumber, &artifact.ObjectKey, &artifact.ContentType, &artifact.SizeBytes, &artifact.SHA256, &artifact.RetentionClass, &artifact.CreatedBy, &artifact.CreatedAt)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("scan artifact: %w", err)
	}
	artifact.CreatedAt = artifact.CreatedAt.UTC()
	return artifact, nil
}

var _ ports.ArtifactRepository = (*ArtifactRepository)(nil)
