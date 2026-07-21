package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/buildinfo"
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
}

// API exposes the Platform API HTTP handler and process readiness state.
type API struct {
	build      buildinfo.Info
	readyState atomic.Bool
	readiness  func(context.Context) error
	handler    http.Handler
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

	api := &API{build: options.Build, readiness: options.Readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", api.live)
	mux.HandleFunc("GET /health/ready", api.ready)

	var handler http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && (request.URL.Path == "/health/live" || request.URL.Path == "/health/ready") {
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

// Handler returns the complete HTTP middleware chain.
func (api *API) Handler() http.Handler {
	return api.handler
}

// SetReady updates process readiness. It is safe to invoke during shutdown.
func (api *API) SetReady(ready bool) {
	api.readyState.Store(ready)
}
