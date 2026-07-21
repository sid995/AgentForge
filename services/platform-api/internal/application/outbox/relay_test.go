package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type fakeRepository struct {
	mu                sync.Mutex
	events            []ports.ClaimedOutboxEvent
	published, failed int
	permanent         bool
	failureReason     string
}

func (repo *fakeRepository) Claim(context.Context, string, time.Time, time.Duration, int) ([]ports.ClaimedOutboxEvent, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	return append([]ports.ClaimedOutboxEvent(nil), repo.events...), nil
}
func (repo *fakeRepository) MarkPublished(context.Context, uuid.UUID, string, time.Time, ports.PublicationResult) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.published++
	return nil
}
func (repo *fakeRepository) MarkFailed(_ context.Context, _ uuid.UUID, _ string, _ time.Time, _, reason string, permanent bool) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.failed++
	repo.permanent = permanent
	repo.failureReason = reason
	return nil
}
func (repo *fakeRepository) CleanupPublished(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

type fakePublisher struct {
	err           error
	calls, closed int
}

func (publisher *fakePublisher) Publish(context.Context, events.OutboxEvent) (ports.PublicationResult, error) {
	publisher.calls++
	return ports.PublicationResult{Partition: 1, Offset: 2}, publisher.err
}
func (publisher *fakePublisher) Close() error { publisher.closed++; return nil }

type permanentPublishError struct{}

func (permanentPublishError) Error() string   { return "malformed outbox payload" }
func (permanentPublishError) Permanent() bool { return true }

type safePublishError struct{ private string }

func (err safePublishError) Error() string   { return err.private }
func (safePublishError) SafeMessage() string { return "broker unavailable" }

func TestRunOnceMarksSuccessAndFailures(t *testing.T) {
	now := time.Date(2026, 7, 21, 16, 0, 0, 0, time.UTC)
	eventID := uuid.Must(uuid.NewV7())
	claimed := ports.ClaimedOutboxEvent{OutboxEvent: events.OutboxEvent{Envelope: events.Envelope{EventID: eventID, EventType: "agent-run.requested.v1"}}}
	for _, test := range []struct {
		name                      string
		publishError              error
		wantPublished, wantFailed int
		wantPermanent             bool
	}{
		{name: "published", wantPublished: 1},
		{name: "broker unavailable", publishError: errors.New("broker unavailable"), wantFailed: 1},
		{name: "malformed", publishError: permanentPublishError{}, wantFailed: 1, wantPermanent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{events: []ports.ClaimedOutboxEvent{claimed}}
			publisher := &fakePublisher{err: test.publishError}
			relay, _ := NewRelay(repository, publisher, nil, "relay-1", func() time.Time { return now })
			count, err := relay.RunOnce(context.Background())
			if err != nil || count != 1 || repository.published != test.wantPublished || repository.failed != test.wantFailed || repository.permanent != test.wantPermanent {
				t.Fatalf("RunOnce() count=%d err=%v repo=%#v", count, err, repository)
			}
		})
	}
}

func TestRunOncePersistsOnlySafeFailureMessages(t *testing.T) {
	eventID := uuid.Must(uuid.NewV7())
	repository := &fakeRepository{events: []ports.ClaimedOutboxEvent{{OutboxEvent: events.OutboxEvent{Envelope: events.Envelope{EventID: eventID, EventType: "agent-run.requested.v1"}}}}}
	publisher := &fakePublisher{err: errors.New("password=do-not-store")}
	relay, _ := NewRelay(repository, publisher, nil, "relay-1", time.Now)
	if _, err := relay.RunOnce(context.Background()); err != nil || repository.failureReason != "event publication failed" {
		t.Fatalf("unsafe fallback reason=%q err=%v", repository.failureReason, err)
	}

	repository.failureReason = ""
	publisher.err = safePublishError{private: "password=still-private"}
	if _, err := relay.RunOnce(context.Background()); err != nil || repository.failureReason != "broker unavailable" {
		t.Fatalf("safe reason=%q err=%v", repository.failureReason, err)
	}
}

func TestRunClosesPublisherOnCancellation(t *testing.T) {
	repository := &fakeRepository{}
	publisher := &fakePublisher{}
	relay, _ := NewRelay(repository, publisher, nil, "relay-1", time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := relay.Run(ctx); err != nil || publisher.closed != 1 {
		t.Fatalf("Run() err=%v closed=%d", err, publisher.closed)
	}
}
