package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/application/runs"
	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/identity"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

const jsonContentType = "application/json; charset=utf-8"

// Options configures the HTTP transport. It contains only transport concerns
// so later application ports remain independent of net/http.
type Options struct {
	Logger              *slog.Logger
	Build               buildinfo.Info
	RequestTimeout      time.Duration
	MaxRequestBodyBytes int64
	RequestIDGenerator  func() (string, error)
	Readiness           func(context.Context) error
	IdentityResolver    identity.Resolver
	Runs                RunService
}

// RunService is the application boundary owned by AgentRun HTTP routes.
type RunService interface {
	Create(context.Context, identity.Identity, uuid.UUID, runs.CreateInput) (domain.AgentRun, bool, error)
	Get(context.Context, identity.Identity, uuid.UUID) (domain.AgentRun, error)
	List(context.Context, identity.Identity, uuid.UUID, ports.AgentRunPage) ([]domain.AgentRun, error)
}

// API exposes the Platform API HTTP handler and process readiness state.
type API struct {
	build            buildinfo.Info
	readyState       atomic.Bool
	readiness        func(context.Context) error
	identityResolver identity.Resolver
	runs             RunService
	handler          http.Handler
}

// New constructs an API that is initially not ready. The process entrypoint
// marks it ready only after binding its listener.
func New(options Options) *API {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 30 * time.Second
	}
	if options.MaxRequestBodyBytes <= 0 {
		options.MaxRequestBodyBytes = 1 << 20
	}
	if options.RequestIDGenerator == nil {
		options.RequestIDGenerator = newRequestID
	}

	api := &API{build: options.Build, readiness: options.Readiness, identityResolver: options.IdentityResolver, runs: options.Runs}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", api.live)
	mux.HandleFunc("GET /health/ready", api.ready)
	if api.runs != nil {
		mux.HandleFunc("POST /v1/projects/{projectId}/runs", api.createRun)
		mux.HandleFunc("GET /v1/projects/{projectId}/runs", api.listRuns)
		mux.HandleFunc("GET /v1/runs/{runId}", api.getRun)
	}

	var handler http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if isHandledRoute(request, api.runs != nil) {
			mux.ServeHTTP(writer, request)
			return
		}
		writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "The requested resource was not found.", false)
	})
	handler = bodyLimit(options.MaxRequestBodyBytes, handler)
	handler = recoverPanic(options.Logger, handler)
	handler = timeout(options.RequestTimeout, handler)
	handler = requestLogging(options.Logger, handler)
	handler = requestContext(options.RequestIDGenerator, handler)
	api.handler = handler
	return api
}

func isHandledRoute(request *http.Request, runsEnabled bool) bool {
	if request.Method == http.MethodGet && (request.URL.Path == "/health/live" || request.URL.Path == "/health/ready") {
		return true
	}
	if !runsEnabled {
		return false
	}
	parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(parts) == 3 && parts[0] == "v1" && parts[1] == "runs" {
		return request.Method == http.MethodGet
	}
	return len(parts) == 4 && parts[0] == "v1" && parts[1] == "projects" && parts[3] == "runs" && (request.Method == http.MethodGet || request.Method == http.MethodPost)
}

// Handler returns the complete HTTP middleware chain.
func (api *API) Handler() http.Handler {
	return api.handler
}

// SetReady updates process readiness. It is safe to invoke during shutdown.
func (api *API) SetReady(ready bool) {
	api.readyState.Store(ready)
}
