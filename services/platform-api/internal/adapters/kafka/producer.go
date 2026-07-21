package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type syncProducer interface {
	ProduceSync(context.Context, ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer is the Kafka-specific implementation of the event publisher port.
type Producer struct {
	client          syncProducer
	maxMessageBytes int
}

func NewProducer(configuration config.KafkaConfig) (*Producer, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(configuration.Brokers...),
		kgo.ClientID(configuration.ClientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(configuration.MaxAttempts-1),
		kgo.UnknownTopicRetries(1),
		kgo.RecordDeliveryTimeout(configuration.PublishTimeout),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &Producer{client: client, maxMessageBytes: configuration.MaxMessageBytes}, nil
}

// Publish validates the provider-neutral event before producing synchronously.
func (producer *Producer) Publish(ctx context.Context, event events.OutboxEvent) (ports.PublicationResult, error) {
	if len(event.Serialized) == 0 || len(event.Serialized) > producer.maxMessageBytes {
		return ports.PublicationResult{}, &PublicationError{permanent: true, reason: "event message size is invalid"}
	}
	decoded, err := events.DecodeEnvelope(event.Serialized)
	if err != nil || decoded.EventID != event.Envelope.EventID || event.Topic != events.AgentRunLifecycleTopic || event.PartitionKey != decoded.AggregateID.String() {
		return ports.PublicationResult{}, &PublicationError{cause: err, permanent: true, reason: "event contract is invalid"}
	}
	headers := events.HeadersForEnvelope(decoded)
	record := &kgo.Record{Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Serialized, Timestamp: decoded.OccurredAt}
	for _, header := range headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: header.Key, Value: []byte(header.Value)})
	}
	result := producer.client.ProduceSync(ctx, record)[0]
	if result.Err != nil {
		return ports.PublicationResult{}, classifyPublicationError(result.Err)
	}
	return ports.PublicationResult{Partition: int(result.Record.Partition), Offset: result.Record.Offset}, nil
}

func (producer *Producer) Close() error {
	producer.client.Close()
	return nil
}

var _ ports.EventPublisher = (*Producer)(nil)
