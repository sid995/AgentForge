package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/application/runs"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestCreateRunRequiresServerConfiguredBearerIdentity(t *testing.T) {
	api := runTestAPI(t, &fakeRunService{})
	projectID := uuid.Must(uuid.NewV7())
	response := serve(api.Handler(), http.MethodPost, "/v1/projects/"+projectID.String()+"/runs", bytes.NewBufferString(`{}`))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("create status=%d, want %d", response.Code, http.StatusUnauthorized)
	}
	assertErrorCode(t, response, "AUTHENTICATION_REQUIRED")
}

func TestCreateRunAcceptsSecureReferenceAndDoesNotReturnIt(t *testing.T) {
	service := &fakeRunService{run: testRun()}
	api := runTestAPI(t, service)
	request := httptest.NewRequest(http.MethodPost, "/v1/projects/"+service.run.ProjectID.String()+"/runs", bytes.NewBufferString(`{"promptRef":"vault://prompts/private-request","runtime":"python-3.12","resources":{"cpuMillis":1000,"memoryMiB":2048},"timeoutSeconds":1800,"maxAttempts":3}`))
	request.Header.Set("Authorization", "Bearer local-token")
	request.Header.Set("Idempotency-Key", "create-key-1")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || service.create.ProjectID != service.run.ProjectID || service.create.Input.IdempotencyKey != "create-key-1" || service.create.Caller.TenantID != service.tenantID {
		t.Fatalf("create response=%d call=%#v", response.Code, service.create)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("promptRef")) || bytes.Contains(response.Body.Bytes(), []byte("private-request")) {
		t.Fatalf("create response leaked prompt reference: %s", response.Body.String())
	}
	if response.Header().Get("Location") != "/v1/runs/"+service.run.ID.String() {
		t.Fatalf("Location=%q", response.Header().Get("Location"))
	}
}

func TestCreateRunRejectsUnknownFieldsAndReusedKey(t *testing.T) {
	service := &fakeRunService{run: testRun()}
	api := runTestAPI(t, service)
	projectPath := "/v1/projects/" + service.run.ProjectID.String() + "/runs"
	unknown := httptest.NewRequest(http.MethodPost, projectPath, bytes.NewBufferString(`{"prompt":"raw prompt"}`))
	unknown.Header.Set("Authorization", "Bearer local-token")
	unknown.Header.Set("Idempotency-Key", "create-key-1")
	unknownResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status=%d", unknownResponse.Code)
	}
	assertErrorCode(t, unknownResponse, "VALIDATION")

	service.createErr = runs.ErrIdempotencyKeyReused
	conflict := httptest.NewRequest(http.MethodPost, projectPath, bytes.NewBufferString(`{"promptRef":"vault://prompts/request","runtime":"python-3.12","resources":{"cpuMillis":1000,"memoryMiB":2048},"timeoutSeconds":1800,"maxAttempts":3}`))
	conflict.Header.Set("Authorization", "Bearer local-token")
	conflict.Header.Set("Idempotency-Key", "create-key-1")
	conflictResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("key reuse status=%d", conflictResponse.Code)
	}
	assertErrorCode(t, conflictResponse, "IDEMPOTENCY_KEY_REUSED")
}

func TestGetAndListRunsAreTenantScopedAndCursorPaginated(t *testing.T) {
	service := &fakeRunService{run: testRun()}
	api := runTestAPI(t, service)
	get := httptest.NewRequest(http.MethodGet, "/v1/runs/"+service.run.ID.String(), nil)
	get.Header.Set("Authorization", "Bearer local-token")
	getResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || service.get.Caller.TenantID != service.tenantID {
		t.Fatalf("get status=%d call=%#v", getResponse.Code, service.get)
	}

	service.items = []domain.AgentRun{service.run}
	list := httptest.NewRequest(http.MethodGet, "/v1/projects/"+service.run.ProjectID.String()+"/runs?limit=1", nil)
	list.Header.Set("Authorization", "Bearer local-token")
	listResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || service.list.Page.Limit != 1 || service.list.Caller.TenantID != service.tenantID {
		t.Fatalf("list status=%d call=%#v", listResponse.Code, service.list)
	}
	var decoded runsResponse
	if err := json.Unmarshal(listResponse.Body.Bytes(), &decoded); err != nil || len(decoded.Data) != 1 || decoded.NextCursor == "" {
		t.Fatalf("list response=%s error=%v", listResponse.Body.String(), err)
	}
	if _, err := decodeCursor(decoded.NextCursor); err != nil {
		t.Fatalf("next cursor error=%v", err)
	}
}

func TestCancelRunAcceptsReasonAndReturnsStableCommandResponse(t *testing.T) {
	service := &fakeRunService{run: testRun()}
	service.run.Status = domain.AgentRunCancelling
	api := runTestAPI(t, service)
	request := httptest.NewRequest(http.MethodPost, "/v1/runs/"+service.run.ID.String()+"/cancel", bytes.NewBufferString(`{"reason":"stop after current safe point"}`))
	request.Header.Set("Authorization", "Bearer local-token")
	request.Header.Set("Idempotency-Key", "cancel-key-1")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || service.cancel.RunID != service.run.ID || service.cancel.CommandID != "cancel-key-1" || service.cancel.Reason != "stop after current safe point" || service.cancel.Caller.TenantID != service.tenantID {
		t.Fatalf("cancel response=%d call=%#v", response.Code, service.cancel)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("safe point")) {
		t.Fatalf("cancel response leaked reason: %s", response.Body.String())
	}
}

func TestRetryRunRequiresEmptyObjectAndMapsCommandConflicts(t *testing.T) {
	service := &fakeRunService{run: testRun()}
	service.run.Status = domain.AgentRunQueued
	service.run.AttemptCount = 2
	api := runTestAPI(t, service)
	path := "/v1/runs/" + service.run.ID.String() + "/retry"
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	request.Header.Set("Authorization", "Bearer local-token")
	request.Header.Set("Idempotency-Key", "retry-key-1")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || service.retry.RunID != service.run.ID || service.retry.CommandID != "retry-key-1" || service.retry.Caller.TenantID != service.tenantID {
		t.Fatalf("retry response=%d call=%#v", response.Code, service.retry)
	}

	service.retryErr = domain.ErrRetryNotAllowed
	conflict := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	conflict.Header.Set("Authorization", "Bearer local-token")
	conflict.Header.Set("Idempotency-Key", "retry-key-2")
	conflictResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("retry conflict status=%d", conflictResponse.Code)
	}
	assertErrorCode(t, conflictResponse, "RETRY_NOT_ALLOWED")
}

func TestRunErrorUsesStructuralValidationClassification(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/invalid", nil)
	request = request.WithContext(context.WithValue(request.Context(), requestIDKey, "req_test"))
	validation := httptest.NewRecorder()
	writeRunError(validation, request, errors.New("wrapped: "+domain.ErrValidation.Error()))
	if validation.Code != http.StatusInternalServerError {
		t.Fatalf("unwrapped validation status=%d, want internal", validation.Code)
	}

	classified := httptest.NewRecorder()
	writeRunError(classified, request, fmt.Errorf("bad input: %w", domain.ErrValidation))
	if classified.Code != http.StatusUnprocessableEntity {
		t.Fatalf("classified validation status=%d, want 422", classified.Code)
	}
	assertErrorCode(t, classified, "VALIDATION")
}

func TestRunResponsePreservesNullableLifecycleFields(t *testing.T) {
	run := testRun()
	run.FailureCategory = domain.FailureTransientDependency
	run.CancellationReason = "operator requested"
	run.CancellationRequestedBy = "developer@example.test"
	requestedAt := run.UpdatedAt.Add(time.Minute)
	completedAt := requestedAt.Add(time.Minute)
	run.CancellationRequestedAt = &requestedAt
	run.CompletedAt = &completedAt
	response := responseForRun(run)
	if response.FailureCategory == nil || response.CancellationReason == nil || response.CancellationRequestedAt == nil || response.CompletedAt == nil {
		t.Fatalf("response omitted lifecycle fields: %#v", response)
	}
	unset := responseForRun(testRun())
	if unset.FailureCategory != nil || unset.CancellationReason != nil || unset.CancellationRequestedAt != nil || unset.CompletedAt != nil {
		t.Fatalf("response did not preserve unset fields as null pointers: %#v", unset)
	}
}

func runTestAPI(t *testing.T, service *fakeRunService) *API {
	t.Helper()
	service.tenantID = uuid.Must(uuid.NewV7())
	resolver, err := identity.NewDevelopmentResolver([]identity.DevelopmentCredential{{Token: "local-token", Identity: identity.Identity{TenantID: service.tenantID, Subject: "developer@example.test", Role: identity.RoleDeveloper}}})
	if err != nil {
		t.Fatalf("NewDevelopmentResolver() error=%v", err)
	}
	return New(Options{Build: buildinfo.Info{Version: "test"}, RequestIDGenerator: func() (string, error) { return "req_test", nil }, IdentityResolver: resolver, Runs: service})
}

func testRun() domain.AgentRun {
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	return domain.AgentRun{ID: uuid.Must(uuid.NewV7()), ProjectID: uuid.Must(uuid.NewV7()), Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 3, AttemptCount: 1, Status: domain.AgentRunQueued, CreatedAt: now, UpdatedAt: now}
}

type runCreateCall struct {
	Caller    identity.Identity
	ProjectID uuid.UUID
	Input     runs.CreateInput
}
type runGetCall struct {
	Caller identity.Identity
	RunID  uuid.UUID
}
type runListCall struct {
	Caller    identity.Identity
	ProjectID uuid.UUID
	Page      ports.AgentRunPage
}
type runCancelCall struct {
	Caller    identity.Identity
	RunID     uuid.UUID
	CommandID string
	Reason    string
}
type runRetryCall struct {
	Caller    identity.Identity
	RunID     uuid.UUID
	CommandID string
}
type fakeRunService struct {
	tenantID  uuid.UUID
	run       domain.AgentRun
	items     []domain.AgentRun
	create    runCreateCall
	get       runGetCall
	list      runListCall
	cancel    runCancelCall
	retry     runRetryCall
	createErr error
	cancelErr error
	retryErr  error
}

func (service *fakeRunService) Create(_ context.Context, caller identity.Identity, projectID uuid.UUID, input runs.CreateInput) (domain.AgentRun, bool, error) {
	service.create = runCreateCall{Caller: caller, ProjectID: projectID, Input: input}
	if service.createErr != nil {
		return domain.AgentRun{}, false, service.createErr
	}
	return service.run, false, nil
}
func (service *fakeRunService) Get(_ context.Context, caller identity.Identity, runID uuid.UUID) (domain.AgentRun, error) {
	service.get = runGetCall{Caller: caller, RunID: runID}
	if runID != service.run.ID {
		return domain.AgentRun{}, domain.ErrNotFound
	}
	return service.run, nil
}
func (service *fakeRunService) List(_ context.Context, caller identity.Identity, projectID uuid.UUID, page ports.AgentRunPage) ([]domain.AgentRun, error) {
	service.list = runListCall{Caller: caller, ProjectID: projectID, Page: page}
	if projectID != service.run.ProjectID {
		return nil, domain.ErrNotFound
	}
	return service.items, nil
}
func (service *fakeRunService) Cancel(_ context.Context, caller identity.Identity, runID uuid.UUID, commandID, reason string) (ports.RunCommandResult, error) {
	service.cancel = runCancelCall{Caller: caller, RunID: runID, CommandID: commandID, Reason: reason}
	if service.cancelErr != nil {
		return ports.RunCommandResult{}, service.cancelErr
	}
	return ports.RunCommandResult{Run: service.run}, nil
}
func (service *fakeRunService) Retry(_ context.Context, caller identity.Identity, runID uuid.UUID, commandID string) (ports.RunCommandResult, error) {
	service.retry = runRetryCall{Caller: caller, RunID: runID, CommandID: commandID}
	if service.retryErr != nil {
		return ports.RunCommandResult{}, service.retryErr
	}
	return ports.RunCommandResult{Run: service.run}, nil
}

var _ RunService = (*fakeRunService)(nil)
