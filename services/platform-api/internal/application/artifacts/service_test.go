package artifacts

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestListAndGetRequireCallerOwnedRun(t *testing.T) {
	fixture := artifactFixture(t)
	runs := &fakeRunReader{run: domain.AgentRun{ID: fixture.RunID}}
	metadata := &fakeMetadataRepository{artifact: fixture, artifacts: []domain.Artifact{fixture}}
	service := NewService(runs, metadata, nil)
	caller := identity.Identity{TenantID: fixture.TenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}

	items, err := service.List(context.Background(), caller, fixture.RunID)
	if err != nil || len(items) != 1 || metadata.listTenantID != caller.TenantID || runs.tenantID != caller.TenantID {
		t.Fatalf("list=%#v error=%v runs=%#v metadata=%#v", items, err, runs, metadata)
	}
	got, err := service.Get(context.Background(), caller, fixture.RunID, fixture.ID)
	if err != nil || got.ID != fixture.ID || metadata.getTenantID != caller.TenantID {
		t.Fatalf("get=%#v error=%v metadata=%#v", got, err, metadata)
	}

	metadata.artifact.RunID = uuid.Must(uuid.NewV7())
	if _, err := service.Get(context.Background(), caller, fixture.RunID, fixture.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-run artifact error=%v, want ErrNotFound", err)
	}
}

func TestDownloadUsesTrustedMetadataKeyAndBoundedLifetime(t *testing.T) {
	fixture := artifactFixture(t)
	store := &fakeArtifactStore{download: ports.PresignedDownload{URL: mustURL(t, "https://storage.example.test/download?signature=opaque"), ExpiresAt: fixture.CreatedAt.Add(5 * time.Minute)}}
	service := NewService(&fakeRunReader{run: domain.AgentRun{ID: fixture.RunID}}, &fakeMetadataRepository{artifact: fixture}, store)
	caller := identity.Identity{TenantID: fixture.TenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}

	download, err := service.Download(context.Background(), caller, fixture.RunID, fixture.ID, 5*time.Minute)
	if err != nil || download.URL.String() != store.download.URL.String() || store.key.TenantID != fixture.TenantID || store.key.ProjectID != fixture.ProjectID || store.key.RunID != fixture.RunID || store.key.Attempt != fixture.AttemptNumber || store.key.Name != "logs/runner.jsonl" || store.lifetime != 5*time.Minute {
		t.Fatalf("download=%#v error=%v store=%#v", download, err, store)
	}
	if _, err := service.Download(context.Background(), caller, fixture.RunID, fixture.ID, ports.MaxArtifactPresignLifetime+time.Second); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid lifetime error=%v, want validation", err)
	}
	if _, err := NewService(&fakeRunReader{run: domain.AgentRun{ID: fixture.RunID}}, &fakeMetadataRepository{artifact: fixture}, nil).Download(context.Background(), caller, fixture.RunID, fixture.ID, time.Minute); !errors.Is(err, ErrDownloadUnavailable) {
		t.Fatalf("missing store error=%v, want unavailable", err)
	}
	corrupt := fixture
	corrupt.ObjectKey = "tenants/other/projects/other/runs/other/attempts/1/logs/runner.jsonl"
	store = &fakeArtifactStore{download: ports.PresignedDownload{URL: mustURL(t, "https://storage.example.test/download"), ExpiresAt: fixture.CreatedAt.Add(time.Minute)}}
	if _, err := NewService(&fakeRunReader{run: domain.AgentRun{ID: fixture.RunID}}, &fakeMetadataRepository{artifact: corrupt}, store).Download(context.Background(), caller, fixture.RunID, fixture.ID, time.Minute); err == nil || store.key.Name != "" {
		t.Fatalf("corrupt metadata download error=%v store=%#v", err, store)
	}
}

func artifactFixture(t *testing.T) domain.Artifact {
	t.Helper()
	tenantID, projectID, runID, attemptID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	artifact, err := domain.NewArtifact(domain.NewArtifactInput{TenantID: tenantID, ProjectID: projectID, RunID: runID, AttemptID: attemptID, AttemptNumber: 1, ObjectKey: "tenants/" + tenantID.String() + "/projects/" + projectID.String() + "/runs/" + runID.String() + "/attempts/1/logs/runner.jsonl", ContentType: "application/x-ndjson", SizeBytes: 64, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", RetentionClass: domain.ArtifactRetentionHot, CreatedBy: "agent-runner"}, time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new artifact: %v", err)
	}
	return artifact
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return parsed
}

type fakeRunReader struct {
	tenantID uuid.UUID
	runID    uuid.UUID
	run      domain.AgentRun
	err      error
}

func (reader *fakeRunReader) Get(_ context.Context, tenantID, runID uuid.UUID) (domain.AgentRun, error) {
	reader.tenantID, reader.runID = tenantID, runID
	if reader.err != nil || runID != reader.run.ID {
		if reader.err != nil {
			return domain.AgentRun{}, reader.err
		}
		return domain.AgentRun{}, domain.ErrNotFound
	}
	return reader.run, nil
}

type fakeMetadataRepository struct {
	getTenantID  uuid.UUID
	listTenantID uuid.UUID
	artifact     domain.Artifact
	artifacts    []domain.Artifact
}

func (repository *fakeMetadataRepository) Get(_ context.Context, tenantID, artifactID uuid.UUID) (domain.Artifact, error) {
	repository.getTenantID = tenantID
	if artifactID != repository.artifact.ID {
		return domain.Artifact{}, domain.ErrNotFound
	}
	return repository.artifact, nil
}

func (repository *fakeMetadataRepository) ListByRun(_ context.Context, tenantID, _ uuid.UUID) ([]domain.Artifact, error) {
	repository.listTenantID = tenantID
	return repository.artifacts, nil
}

type fakeArtifactStore struct {
	key      ports.ArtifactKey
	lifetime time.Duration
	download ports.PresignedDownload
	err      error
}

func (store *fakeArtifactStore) Put(context.Context, ports.ArtifactKey, ports.ArtifactUpload) (ports.ArtifactObject, error) {
	return ports.ArtifactObject{}, errors.New("not implemented")
}
func (store *fakeArtifactStore) Get(context.Context, ports.ArtifactKey) (ports.ArtifactObject, io.ReadCloser, error) {
	return ports.ArtifactObject{}, nil, errors.New("not implemented")
}
func (store *fakeArtifactStore) Head(context.Context, ports.ArtifactKey) (ports.ArtifactObject, error) {
	return ports.ArtifactObject{}, errors.New("not implemented")
}
func (store *fakeArtifactStore) Delete(context.Context, ports.ArtifactKey) error {
	return errors.New("not implemented")
}
func (store *fakeArtifactStore) PresignDownload(_ context.Context, key ports.ArtifactKey, lifetime time.Duration) (ports.PresignedDownload, error) {
	store.key, store.lifetime = key, lifetime
	return store.download, store.err
}

var _ ports.ArtifactStore = (*fakeArtifactStore)(nil)
