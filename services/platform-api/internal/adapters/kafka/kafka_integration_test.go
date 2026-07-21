//go:build brokerintegration

package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
)

func TestRedpandaProducerConsumerOrderingHeadersAndDuplicateDelivery(t *testing.T) {
	brokers := os.Getenv("AGENTFORGE_TEST_KAFKA_BROKERS")
	if brokers == "" {
		t.Fatal("AGENTFORGE_TEST_KAFKA_BROKERS is required")
	}
	configuration, err := config.LoadKafka(func(name string) (string, bool) {
		if name == "AGENTFORGE_KAFKA_BROKERS" {
			return brokers, true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	producer, err := NewProducer(configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	consumer, err := NewConsumer(configuration, "contract-"+uuid.NewString(), events.AgentRunLifecycleTopic)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	event := fixtureEvent(t)
	event.Envelope.OccurredAt = time.Now().UTC()
	event.CreatedAt = event.Envelope.OccurredAt
	event.Serialized, err = json.Marshal(event.Envelope)
	if err != nil {
		t.Fatalf("serialize current event: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := producer.Publish(ctx, event); err != nil {
		t.Fatalf("first publish: %v (cause: %v)", err, errors.Unwrap(err))
	}
	if _, err := producer.Publish(ctx, event); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	first, err := consumer.Poll(ctx)
	if err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if err := consumer.Acknowledge(ctx, first); err != nil {
		t.Fatalf("first acknowledge: %v", err)
	}
	second, err := consumer.Poll(ctx)
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if first.Envelope.EventID != event.Envelope.EventID || second.Envelope.EventID != event.Envelope.EventID || first.Partition != second.Partition || first.Offset >= second.Offset {
		t.Fatalf("records out of order or identity changed: first=%#v second=%#v", first, second)
	}
}
