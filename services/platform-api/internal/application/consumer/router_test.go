package consumer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type capturingPublisher struct {
	events []events.OutboxEvent
	err    error
}

func (publisher *capturingPublisher) Publish(_ context.Context, event events.OutboxEvent) (ports.PublicationResult, error) {
	publisher.events = append(publisher.events, event)
	return ports.PublicationResult{}, publisher.err
}
func (*capturingPublisher) Close() error { return nil }

func TestRouterPublishesRetriesThenDLQWithoutChangingOriginalIdentity(t *testing.T) {
	source := routerFixture(t)
	publisher := &capturingPublisher{}
	router, err := NewRouter(publisher, nil, "scheduler.v1", uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	transient := NewTransientFailure("DEPENDENCY_TIMEOUT", "dependency unavailable", errors.New("password=private"))
	routed, err := router.Route(context.Background(), source, transient, 1, now, now)
	if err != nil || routed != RoutedRetry || len(publisher.events) != 1 {
		t.Fatalf("routed=%q events=%d err=%v", routed, len(publisher.events), err)
	}
	retry := publisher.events[0]
	if retry.Envelope.EventID != source.Envelope.EventID || retry.Topic != retryTopics[0] || string(retry.Serialized) != string(source.Value) {
		t.Fatalf("retry changed original=%#v", retry)
	}
	receivedRetry := source
	receivedRetry.Topic = retry.Topic
	receivedRetry.Headers = append(events.HeadersForEnvelope(source.Envelope), retry.Headers...)
	notBefore, err := RetryNotBefore(receivedRetry)
	if err != nil || !notBefore.Equal(now.Add(time.Minute)) {
		t.Fatalf("retry not-before=%s err=%v", notBefore, err)
	}
	exhausted := source
	exhausted.Topic = retryTopics[2]
	exhausted.Headers = append(events.HeadersForEnvelope(source.Envelope), events.Header{Key: "delivery-attempt", Value: "3"}, events.Header{Key: "retry-not-before", Value: now.Format(time.RFC3339Nano)})
	routed, err = router.Route(context.Background(), exhausted, transient, 4, now, now.Add(time.Minute))
	if err != nil || routed != RoutedDLQ || len(publisher.events) != 2 {
		t.Fatalf("routed=%q events=%d err=%v", routed, len(publisher.events), err)
	}
	dlq := publisher.events[1]
	if dlq.Envelope.EventID == source.Envelope.EventID || dlq.Topic != events.AgentRunDLQTopic || !strings.Contains(string(dlq.Serialized), source.Envelope.EventID.String()) || strings.Contains(string(dlq.Serialized), "password=private") {
		t.Fatalf("invalid DLQ=%s", dlq.Serialized)
	}
}

func TestRouterPermanentlyQuarantinesMalformedBytesByFingerprint(t *testing.T) {
	publisher := &capturingPublisher{}
	router, _ := NewRouter(publisher, nil, "scheduler.v1", uuid.Must(uuid.NewV7()))
	now := time.Now().UTC()
	source := ports.ReceivedEvent{Topic: events.AgentRunLifecycleTopic, Partition: 1, Offset: 9, Value: []byte(`{"authorization":"secret-value"}`), Headers: []events.Header{{Key: "authorization", Value: "secret-value"}}}
	routed, err := router.Route(context.Background(), source, NewPermanentFailure("INVALID_ENVELOPE", "invalid event envelope", errors.New("secret-value")), 1, now, now)
	if err != nil || routed != RoutedDLQ || len(publisher.events) != 1 {
		t.Fatalf("routed=%q events=%d err=%v", routed, len(publisher.events), err)
	}
	serialized := string(publisher.events[0].Serialized)
	if strings.Contains(serialized, "secret-value") || strings.Contains(serialized, "authorization") || !strings.Contains(serialized, "sha256:") {
		t.Fatalf("malformed bytes leaked into DLQ: %s", serialized)
	}
}

func TestRouterDoesNotReportSuccessWhenRetryPublicationFails(t *testing.T) {
	publisher := &capturingPublisher{err: errors.New("broker unavailable")}
	router, _ := NewRouter(publisher, nil, "scheduler.v1", uuid.Must(uuid.NewV7()))
	now := time.Now().UTC()
	if routed, err := router.Route(context.Background(), routerFixture(t), errors.New("temporary"), 1, now, now); err == nil || routed != "" {
		t.Fatalf("routed=%q err=%v", routed, err)
	}
}

func routerFixture(t *testing.T) ports.ReceivedEvent {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	serialized, err := os.ReadFile(filepath.Join(root, "contracts", "events", "examples", "agent-run.requested.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := events.DecodeEnvelope(serialized)
	if err != nil {
		t.Fatal(err)
	}
	return ports.ReceivedEvent{Envelope: envelope, Topic: events.AgentRunLifecycleTopic, Partition: 1, Offset: 8, Key: []byte(envelope.AggregateID.String()), Headers: events.HeadersForEnvelope(envelope), Value: serialized}
}
