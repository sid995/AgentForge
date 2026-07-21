package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/logging"
)

func TestHealthEndpoints(t *testing.T) {
	api := newTestAPI(t)

	live := serve(api.Handler(), http.MethodGet, "/health/live", nil)
	if live.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want %d", live.Code, http.StatusOK)
	}
	assertHealthResponse(t, live, "build-1")

	notReady := serve(api.Handler(), http.MethodGet, "/health/ready", nil)
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready status = %d, want %d", notReady.Code, http.StatusServiceUnavailable)
	}
	assertErrorCode(t, notReady, "NOT_READY")

	api.SetReady(true)
	ready := serve(api.Handler(), http.MethodGet, "/health/ready", nil)
	if ready.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", ready.Code, http.StatusOK)
	}
	assertHealthResponse(t, ready, "build-1")
}

func TestRequestIDAndTraceparent(t *testing.T) {
	api := newTestAPI(t)
	request := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	request.Header.Set(requestIDHeader, "external-request-7")
	trace := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	request.Header.Set(traceparent, trace)
	response := httptest.NewRecorder()

	api.Handler().ServeHTTP(response, request)
	if got := response.Header().Get(requestIDHeader); got != "external-request-7" {
		t.Fatalf("request ID = %q", got)
	}
	if got := response.Header().Get(traceparent); got != trace {
		t.Fatalf("traceparent = %q", got)
	}

	generated := serve(api.Handler(), http.MethodGet, "/unknown", nil)
	if got := generated.Header().Get(requestIDHeader); got != "req_test" {
		t.Fatalf("generated request ID = %q", got)
	}
}

func TestErrorsAreJSONAndBounded(t *testing.T) {
	api := newTestAPI(t)
	notFound := serve(api.Handler(), http.MethodGet, "/unknown", nil)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", notFound.Code)
	}
	assertErrorCode(t, notFound, "NOT_FOUND")
	if got := notFound.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("content type = %q", got)
	}
	wrongMethod := serve(api.Handler(), http.MethodPost, "/health/live", nil)
	if wrongMethod.Code != http.StatusNotFound {
		t.Fatalf("wrong method status = %d", wrongMethod.Code)
	}
	assertErrorCode(t, wrongMethod, "NOT_FOUND")

	overLimit := httptest.NewRequest(http.MethodPost, "/unknown", strings.NewReader("12345"))
	overLimit.ContentLength = 5
	response := httptest.NewRecorder()
	New(Options{MaxRequestBodyBytes: 4, RequestIDGenerator: func() (string, error) { return "req_test", nil }}).Handler().ServeHTTP(response, overLimit)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status = %d", response.Code)
	}
	assertErrorCode(t, response, "REQUEST_TOO_LARGE")
}

func TestTimeoutAndPanicRecovery(t *testing.T) {
	timeoutHandler := requestContext(func() (string, error) { return "req_timeout", nil }, timeout(time.Millisecond, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	})))
	timedOut := serve(timeoutHandler, http.MethodGet, "/", nil)
	if timedOut.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status = %d", timedOut.Code)
	}
	assertErrorCode(t, timedOut, "REQUEST_TIMEOUT")

	panicHandler := requestContext(func() (string, error) { return "req_panic", nil }, recoverPanic(slog.New(slog.NewTextHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("prompt contents must not be exposed")
	})))
	recovered := serve(panicHandler, http.MethodGet, "/", nil)
	if recovered.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d", recovered.Code)
	}
	assertErrorCode(t, recovered, "INTERNAL")
	if strings.Contains(recovered.Body.String(), "prompt contents") {
		t.Fatal("panic details leaked in response")
	}
}

func TestRequestLogExcludesSensitiveHeadersAndBody(t *testing.T) {
	var logs bytes.Buffer
	logger := logging.New(&logs, "test")
	handler := requestContext(func() (string, error) { return "req_log", nil }, requestLogging(logger, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})))
	request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("private prompt"))
	request.Header.Set("Authorization", "Bearer secret-token")
	request.Header.Set(traceparent, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	output := logs.String()
	if !strings.Contains(output, "req_log") ||
		!strings.Contains(output, "4bf92f3577b34da6a3ce929d0e0e4736") ||
		!strings.Contains(output, `"service":"platform-api"`) ||
		strings.Contains(output, "secret-token") ||
		strings.Contains(output, "private prompt") {
		t.Fatalf("unexpected log output: %s", output)
	}
}

func TestServerStartsAndStops(t *testing.T) {
	api := newTestAPI(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	server := &http.Server{Handler: api.Handler()}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()

	api.SetReady(true)
	response, err := http.Get("http://" + listener.Addr().String() + "/health/ready")
	if err != nil {
		t.Fatalf("GET ready error = %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET ready status = %d", response.StatusCode)
	}

	api.SetReady(false)
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-stopped; err != http.ErrServerClosed {
		t.Fatalf("Serve() error = %v, want %v", err, http.ErrServerClosed)
	}
}

func newTestAPI(t *testing.T) *API {
	t.Helper()
	return New(Options{
		Build:               buildinfo.Info{Version: "build-1", Commit: "commit-1", BuildTime: "2026-07-21T00:00:00Z"},
		RequestTimeout:      time.Second,
		MaxRequestBodyBytes: 1024,
		RequestIDGenerator:  func() (string, error) { return "req_test", nil },
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func serve(handler http.Handler, method, target string, body io.Reader) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, target, body))
	return response
}

func assertHealthResponse(t *testing.T, response *httptest.ResponseRecorder, version string) {
	t.Helper()
	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}
	if response.Header().Get(requestIDHeader) == "" ||
		body.Status != "ok" ||
		body.Service != "platform-api" ||
		body.Version != version ||
		body.Commit != "commit-1" ||
		body.BuildTime != "2026-07-21T00:00:00Z" {
		t.Fatalf("health body = %#v", body)
	}
}

func assertErrorCode(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != code || body.Error.RequestID == "" {
		t.Fatalf("error body = %#v", body)
	}
}
