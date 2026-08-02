// Package artifacts implements tenant-scoped artifact metadata and controlled download access.
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

var (
	ErrDownloadUnavailable = errors.New("artifact download is unavailable")
	ErrDeletionUnavailable = errors.New("artifact deletion is unavailable")
)

type runReader interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.AgentRun, error)
}

type metadataRepository interface {
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.Artifact, error)
	ListByRun(context.Context, uuid.UUID, uuid.UUID) ([]domain.Artifact, error)
	RecordDeletion(context.Context, domain.ArtifactDeletion) error
}

// Service authorizes artifact metadata reads and server-generated downloads.
type Service struct {
	runs      runReader
	artifacts metadataRepository
	store     ports.ArtifactStore
	now       func() time.Time
}

func NewService(runs runReader, artifacts metadataRepository, store ports.ArtifactStore, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{runs: runs, artifacts: artifacts, store: store, now: now}
}

// List returns only artifact metadata belonging to one caller-owned run.
func (service *Service) List(ctx context.Context, caller identity.Identity, runID uuid.UUID) ([]domain.Artifact, error) {
	if !caller.CanManageRuns() {
		return nil, identity.ErrUnauthorized
	}
	if _, err := service.runs.Get(ctx, caller.TenantID, runID); err != nil {
		return nil, err
	}
	return service.artifacts.ListByRun(ctx, caller.TenantID, runID)
}

// Get returns one artifact only when it belongs to the requested caller-owned run.
func (service *Service) Get(ctx context.Context, caller identity.Identity, runID, artifactID uuid.UUID) (domain.Artifact, error) {
	if !caller.CanManageRuns() {
		return domain.Artifact{}, identity.ErrUnauthorized
	}
	return service.artifactForRun(ctx, caller.TenantID, runID, artifactID)
}

// Download issues a bounded read capability only after metadata authorization.
func (service *Service) Download(ctx context.Context, caller identity.Identity, runID, artifactID uuid.UUID, lifetime time.Duration) (ports.PresignedDownload, error) {
	if !caller.CanManageRuns() {
		return ports.PresignedDownload{}, identity.ErrUnauthorized
	}
	if lifetime <= 0 || lifetime > ports.MaxArtifactPresignLifetime {
		return ports.PresignedDownload{}, fmt.Errorf("%w: artifact download lifetime is invalid", domain.ErrValidation)
	}
	artifact, err := service.artifactForRun(ctx, caller.TenantID, runID, artifactID)
	if err != nil {
		return ports.PresignedDownload{}, err
	}
	if service.store == nil {
		return ports.PresignedDownload{}, ErrDownloadUnavailable
	}
	key, err := keyForArtifact(artifact)
	if err != nil {
		return ports.PresignedDownload{}, err
	}
	download, err := service.store.PresignDownload(ctx, key, lifetime)
	if errors.Is(err, ports.ErrArtifactNotFound) {
		return ports.PresignedDownload{}, domain.ErrNotFound
	}
	if errors.Is(err, ports.ErrPresigningUnsupported) {
		return ports.PresignedDownload{}, ErrDownloadUnavailable
	}
	if err != nil {
		return ports.PresignedDownload{}, fmt.Errorf("presign artifact download: %w", err)
	}
	if download.URL == nil || download.ExpiresAt.IsZero() {
		return ports.PresignedDownload{}, fmt.Errorf("artifact store returned an invalid download capability")
	}
	return download, nil
}

// Delete removes one HOT artifact payload and records an immutable tombstone.
// The payload is deleted first so metadata never claims deletion before it has
// happened. Retrying after a crash safely reconciles an already absent object.
func (service *Service) Delete(ctx context.Context, caller identity.Identity, runID, artifactID uuid.UUID, reason string) error {
	if !caller.CanManageArtifacts() {
		return identity.ErrUnauthorized
	}
	artifact, err := service.artifactForRun(ctx, caller.TenantID, runID, artifactID)
	if err != nil {
		return err
	}
	if artifact.RetentionClass != domain.ArtifactRetentionHot {
		return fmt.Errorf("%w: archived artifacts cannot be manually deleted", domain.ErrForbidden)
	}
	deletion, err := domain.NewArtifactDeletion(artifact.ID, caller.TenantID, caller.Subject, reason, service.now())
	if err != nil {
		return err
	}
	if service.store == nil {
		return ErrDeletionUnavailable
	}
	key, err := keyForArtifact(artifact)
	if err != nil {
		return err
	}
	if err := service.store.Delete(ctx, key); err != nil && !errors.Is(err, ports.ErrArtifactNotFound) {
		return fmt.Errorf("delete artifact payload: %w", err)
	}
	if err := service.artifacts.RecordDeletion(ctx, deletion); err != nil && !errors.Is(err, domain.ErrConflict) {
		return err
	}
	return nil
}

func (service *Service) artifactForRun(ctx context.Context, tenantID, runID, artifactID uuid.UUID) (domain.Artifact, error) {
	if _, err := service.runs.Get(ctx, tenantID, runID); err != nil {
		return domain.Artifact{}, err
	}
	artifact, err := service.artifacts.Get(ctx, tenantID, artifactID)
	if err != nil {
		return domain.Artifact{}, err
	}
	if artifact.RunID != runID {
		return domain.Artifact{}, domain.ErrNotFound
	}
	return artifact, nil
}

func keyForArtifact(artifact domain.Artifact) (ports.ArtifactKey, error) {
	prefix := fmt.Sprintf("tenants/%s/projects/%s/runs/%s/attempts/%d/", artifact.TenantID, artifact.ProjectID, artifact.RunID, artifact.AttemptNumber)
	name, found := strings.CutPrefix(artifact.ObjectKey, prefix)
	if !found {
		return ports.ArtifactKey{}, fmt.Errorf("artifact metadata object key is invalid")
	}
	key := ports.ArtifactKey{TenantID: artifact.TenantID, ProjectID: artifact.ProjectID, RunID: artifact.RunID, Attempt: artifact.AttemptNumber, Name: name}
	canonical, err := key.ObjectName()
	if err != nil || canonical != artifact.ObjectKey {
		return ports.ArtifactKey{}, fmt.Errorf("artifact metadata object key is invalid")
	}
	return key, nil
}
