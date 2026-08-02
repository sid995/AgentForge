package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	artifactapp "github.com/sid995/agentforge/services/platform-api/internal/application/artifacts"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestArtifactRoutesAreTenantScopedAndDoNotExposeStorageMetadata(t *testing.T) {
	service := &fakeArtifactService{artifact: testArtifact()}
	api := artifactTestAPI(t, service)
	base := "/v1/runs/" + service.artifact.RunID.String() + "/artifacts/" + service.artifact.ID.String()

	list := httptest.NewRequest(http.MethodGet, "/v1/runs/"+service.artifact.RunID.String()+"/artifacts", nil)
	list.Header.Set("Authorization", "Bearer local-token")
	listResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || service.listCaller.TenantID != service.tenantID || strings.Contains(listResponse.Body.String(), service.artifact.ObjectKey) || strings.Contains(listResponse.Body.String(), service.artifact.CreatedBy) {
		t.Fatalf("list status=%d call=%#v body=%s", listResponse.Code, service.listCaller, listResponse.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, base, nil)
	get.Header.Set("Authorization", "Bearer local-token")
	getResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || service.getCaller.TenantID != service.tenantID || service.getRunID != service.artifact.RunID || service.getArtifactID != service.artifact.ID {
		t.Fatalf("get status=%d service=%#v", getResponse.Code, service)
	}

	download := httptest.NewRequest(http.MethodGet, base+"/download?expiresInSeconds=120", nil)
	download.Header.Set("Authorization", "Bearer local-token")
	downloadResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || service.downloadCaller.TenantID != service.tenantID || service.downloadLifetime != 2*time.Minute || strings.Contains(downloadResponse.Body.String(), service.artifact.ObjectKey) || strings.Contains(downloadResponse.Body.String(), "server-secret") {
		t.Fatalf("download status=%d service=%#v body=%s", downloadResponse.Code, service, downloadResponse.Body.String())
	}
	var body artifactDownloadResponse
	if err := json.Unmarshal(downloadResponse.Body.Bytes(), &body); err != nil || body.Data.URL != service.download.URL.String() || body.Data.ExpiresAt == "" {
		t.Fatalf("download body=%s error=%v", downloadResponse.Body.String(), err)
	}
}

func TestArtifactRoutesMapScopeValidationAndUnavailableErrors(t *testing.T) {
	service := &fakeArtifactService{artifact: testArtifact()}
	api := artifactTestAPI(t, service)
	base := "/v1/runs/" + service.artifact.RunID.String() + "/artifacts/" + service.artifact.ID.String()

	invalidLifetime := httptest.NewRequest(http.MethodGet, base+"/download?expiresInSeconds=901", nil)
	invalidLifetime.Header.Set("Authorization", "Bearer local-token")
	invalidResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(invalidResponse, invalidLifetime)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid lifetime status=%d", invalidResponse.Code)
	}
	assertErrorCode(t, invalidResponse, "VALIDATION")

	service.getErr = domain.ErrNotFound
	notFound := httptest.NewRequest(http.MethodGet, base, nil)
	notFound.Header.Set("Authorization", "Bearer local-token")
	notFoundResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(notFoundResponse, notFound)
	if notFoundResponse.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d", notFoundResponse.Code)
	}
	assertErrorCode(t, notFoundResponse, "NOT_FOUND")

	service.getErr = nil
	service.downloadErr = artifactapp.ErrDownloadUnavailable
	unavailable := httptest.NewRequest(http.MethodGet, base+"/download", nil)
	unavailable.Header.Set("Authorization", "Bearer local-token")
	unavailableResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(unavailableResponse, unavailable)
	if unavailableResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("unclassified unavailable status=%d", unavailableResponse.Code)
	}
	assertErrorCode(t, unavailableResponse, "DEPENDENCY_UNAVAILABLE")
}

func artifactTestAPI(t *testing.T, service *fakeArtifactService) *API {
	t.Helper()
	service.tenantID = uuid.Must(uuid.NewV7())
	resolver, err := identity.NewDevelopmentResolver([]identity.DevelopmentCredential{{Token: "local-token", Identity: identity.Identity{TenantID: service.tenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}}})
	if err != nil {
		t.Fatalf("NewDevelopmentResolver() error=%v", err)
	}
	return New(Options{Build: buildinfo.Info{Version: "test"}, RequestIDGenerator: func() (string, error) { return "req_test", nil }, IdentityResolver: resolver, Artifacts: service})
}

func testArtifact() domain.Artifact {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	return domain.Artifact{ID: uuid.Must(uuid.NewV7()), TenantID: uuid.Must(uuid.NewV7()), ProjectID: uuid.Must(uuid.NewV7()), RunID: uuid.Must(uuid.NewV7()), AttemptID: uuid.Must(uuid.NewV7()), AttemptNumber: 1, ObjectKey: "tenants/private/projects/private/runs/private/attempts/1/logs/runner.jsonl", ContentType: "application/x-ndjson", SizeBytes: 512, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", RetentionClass: domain.ArtifactRetentionHot, CreatedBy: "agent-runner", CreatedAt: now}
}

type fakeArtifactService struct {
	tenantID         uuid.UUID
	artifact         domain.Artifact
	listCaller       identity.Identity
	getCaller        identity.Identity
	downloadCaller   identity.Identity
	getRunID         uuid.UUID
	getArtifactID    uuid.UUID
	downloadLifetime time.Duration
	getErr           error
	downloadErr      error
	download         ports.PresignedDownload
}

func (service *fakeArtifactService) List(_ context.Context, caller identity.Identity, _ uuid.UUID) ([]domain.Artifact, error) {
	service.listCaller = caller
	return []domain.Artifact{service.artifact}, nil
}
func (service *fakeArtifactService) Get(_ context.Context, caller identity.Identity, runID, artifactID uuid.UUID) (domain.Artifact, error) {
	service.getCaller, service.getRunID, service.getArtifactID = caller, runID, artifactID
	if service.getErr != nil {
		return domain.Artifact{}, service.getErr
	}
	return service.artifact, nil
}
func (service *fakeArtifactService) Download(_ context.Context, caller identity.Identity, runID, artifactID uuid.UUID, lifetime time.Duration) (ports.PresignedDownload, error) {
	service.downloadCaller, service.getRunID, service.getArtifactID, service.downloadLifetime = caller, runID, artifactID, lifetime
	if service.downloadErr != nil {
		return ports.PresignedDownload{}, service.downloadErr
	}
	if service.download.URL == nil {
		service.download.URL = mustArtifactURL("https://storage.example.test/download?signature=opaque")
		service.download.ExpiresAt = time.Date(2026, 8, 2, 12, 5, 0, 0, time.UTC)
	}
	return service.download, nil
}

func mustArtifactURL(value string) *url.URL {
	parsed, err := url.Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

var _ ArtifactService = (*fakeArtifactService)(nil)
