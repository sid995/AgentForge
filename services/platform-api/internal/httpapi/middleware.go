package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	requestIDHeader = "X-Request-ID"
	traceparent     = "traceparent"
	maxRequestIDLen = 128
)

type contextKey string

const (
	requestIDKey contextKey = "request-id"
	traceIDKey   contextKey = "trace-id"
)

func requestContext(generate func() (string, error), next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get(requestIDHeader)
		if !validRequestID(requestID) {
			var err error
			requestID, err = generate()
			if err != nil || !validRequestID(requestID) {
				requestID = "req_fallback"
			}
		}

		requestContext := context.WithValue(request.Context(), requestIDKey, requestID)
		if trace := request.Header.Get(traceparent); validTraceparent(trace) {
			writer.Header().Set(traceparent, trace)
			requestContext = context.WithValue(requestContext, traceIDKey, strings.Split(trace, "-")[1])
		}
		writer.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(writer, request.WithContext(requestContext))
	})
}

func bodyLimit(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ContentLength > limit {
			writeError(writer, request, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body exceeds the allowed size.", false)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, limit)
		next.ServeHTTP(writer, request)
	})
}

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panicked", "request_id", requestIDFromContext(request.Context()), "panic_type", typeName(recovered))
				writeError(writer, request, http.StatusInternalServerError, "INTERNAL", "An internal error occurred.", false)
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func timeout(limit time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		context, cancel := context.WithTimeout(request.Context(), limit)
		defer cancel()

		captured := newBufferedResponseWriter()
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			next.ServeHTTP(captured, request.WithContext(context))
		}()

		select {
		case <-finished:
			captured.copyTo(writer)
		case <-context.Done():
			writeError(writer, request, http.StatusGatewayTimeout, "REQUEST_TIMEOUT", "Request timed out.", true)
		}
	})
}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		capture := &statusResponseWriter{ResponseWriter: writer, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(capture, request)
		logger.Info("request completed",
			"request_id", requestIDFromContext(request.Context()),
			"trace_id", traceIDFromContext(request.Context()),
			"method", request.Method,
			"path", request.URL.Path,
			"status", capture.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func requestIDFromContext(context context.Context) string {
	requestID, _ := context.Value(requestIDKey).(string)
	return requestID
}

func traceIDFromContext(context context.Context) string {
	traceID, _ := context.Value(traceIDKey).(string)
	return traceID
}

func newRequestID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "req_" + hex.EncodeToString(bytes), nil
}

func validRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDLen {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func validTraceparent(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return false
	}
	for _, part := range parts {
		if _, err := hex.DecodeString(part); err != nil {
			return false
		}
	}
	return parts[0] != "ff" && strings.Trim(parts[1], "0") != "" && strings.Trim(parts[2], "0") != ""
}

func typeName(value any) string {
	return fmt.Sprintf("%T", value)
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (writer *statusResponseWriter) WriteHeader(status int) {
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

type bufferedResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
	once   sync.Once
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{header: make(http.Header), status: http.StatusOK}
}

func (writer *bufferedResponseWriter) Header() http.Header { return writer.header }

func (writer *bufferedResponseWriter) Write(value []byte) (int, error) {
	writer.once.Do(func() { writer.status = http.StatusOK })
	return writer.body.Write(value)
}

func (writer *bufferedResponseWriter) WriteHeader(status int) {
	writer.once.Do(func() { writer.status = status })
}

func (writer *bufferedResponseWriter) copyTo(destination http.ResponseWriter) {
	for key, values := range writer.header {
		destination.Header()[key] = append([]string(nil), values...)
	}
	destination.WriteHeader(writer.status)
	_, _ = destination.Write(writer.body.Bytes())
}
