package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	schedulerapp "github.com/sid995/agentforge/services/platform-api/internal/application/scheduler"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestServiceHealthReadinessAndMetrics(t *testing.T) {
	var ready atomic.Bool
	dependencyError := errors.New("database unavailable")
	handler := newServiceHandler(&ready, func(context.Context) error { return dependencyError }, schedulerapp.NewMetrics())
	assertStatus(t, handler, "/health/live", http.StatusOK)
	assertStatus(t, handler, "/health/ready", http.StatusServiceUnavailable)
	ready.Store(true)
	dependencyError = nil
	assertStatus(t, handler, "/health/ready", http.StatusOK)
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "agentforge_scheduler_in_flight") {
		t.Fatalf("metrics status=%d body=%s", response.Code, response.Body.String())
	}
}

type hintConsumer struct {
	event        ports.ReceivedEvent
	acknowledged chan struct{}
	sent         atomic.Bool
}

func (consumer *hintConsumer) Poll(ctx context.Context) (ports.ReceivedEvent, error) {
	if !consumer.sent.Swap(true) {
		return consumer.event, nil
	}
	<-ctx.Done()
	return ports.ReceivedEvent{}, ctx.Err()
}
func (consumer *hintConsumer) Acknowledge(context.Context, ports.ReceivedEvent) error {
	close(consumer.acknowledged)
	return nil
}
func (*hintConsumer) Close() {}

func TestKafkaRequestedHintWakesAndAcknowledges(t *testing.T) {
	consumer := &hintConsumer{event: ports.ReceivedEvent{Envelope: events.Envelope{EventType: events.AgentRunRequestedType}}, acknowledged: make(chan struct{})}
	engine := &schedulerapp.Engine{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runHints(ctx, consumer, engine, slog.New(slog.DiscardHandler))
	select {
	case <-consumer.acknowledged:
	case <-time.After(time.Second):
		t.Fatal("hint was not acknowledged")
	}
}

func assertStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s status=%d want=%d", path, response.Code, want)
	}
}
