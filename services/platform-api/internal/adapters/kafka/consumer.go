package kafka

import (
	"bytes"
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type groupClient interface {
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	CloseAllowingRebalance()
}

// Consumer is a manual-acknowledgement Kafka consumer-group base.
type Consumer struct {
	client          groupClient
	maxMessageBytes int
}

func NewConsumer(configuration config.KafkaConfig, group string, topics ...string) (*Consumer, error) {
	if err := configuration.Validate(); err != nil {
		return nil, err
	}
	if group == "" || len(topics) == 0 {
		return nil, fmt.Errorf("kafka consumer group and topics are required")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(configuration.Brokers...),
		kgo.ClientID(configuration.ClientID),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &Consumer{client: client, maxMessageBytes: configuration.MaxMessageBytes}, nil
}

// Poll returns one validated record and never commits its offset implicitly.
func (consumer *Consumer) Poll(ctx context.Context) (ports.ReceivedEvent, error) {
	fetches := consumer.client.PollRecords(ctx, 1)
	if ctx.Err() != nil {
		return ports.ReceivedEvent{}, ctx.Err()
	}
	if fetchErrors := fetches.Errors(); len(fetchErrors) > 0 {
		return ports.ReceivedEvent{}, fmt.Errorf("poll Kafka: %w", fetchErrors[0].Err)
	}
	iterator := fetches.RecordIter()
	if iterator.Done() {
		return ports.ReceivedEvent{}, fmt.Errorf("kafka poll returned no record")
	}
	return consumer.decodeRecord(iterator.Next())
}

func (consumer *Consumer) decodeRecord(record *kgo.Record) (ports.ReceivedEvent, error) {
	received := ports.ReceivedEvent{Topic: record.Topic, Partition: record.Partition, LeaderEpoch: record.LeaderEpoch, Offset: record.Offset, Key: bytes.Clone(record.Key), Value: bytes.Clone(record.Value)}
	for _, header := range record.Headers {
		received.Headers = append(received.Headers, events.Header{Key: header.Key, Value: string(header.Value)})
	}
	if len(record.Value) == 0 || len(record.Value) > consumer.maxMessageBytes {
		return ports.ReceivedEvent{}, &ports.InvalidEventError{Record: received, Cause: fmt.Errorf("message size is invalid")}
	}
	envelope, err := events.DecodeEnvelope(record.Value)
	if err == nil {
		err = events.ValidateHeaders(envelope, received.Headers)
	}
	if err == nil {
		expectedKey, keyErr := events.ExpectedPartitionKey(envelope)
		if keyErr != nil || !events.TopicAccepts(envelope.EventType, record.Topic) || !bytes.Equal(record.Key, []byte(expectedKey)) {
			err = fmt.Errorf("event topic or partition key is invalid")
		}
	}
	if err != nil {
		return ports.ReceivedEvent{}, &ports.InvalidEventError{Record: received, Cause: err}
	}
	received.Envelope = envelope
	return received, nil
}

// Acknowledge synchronously commits only the supplied record's next offset.
func (consumer *Consumer) Acknowledge(ctx context.Context, event ports.ReceivedEvent) error {
	record := &kgo.Record{Topic: event.Topic, Partition: event.Partition, LeaderEpoch: event.LeaderEpoch, Offset: event.Offset}
	if err := consumer.client.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("acknowledge Kafka event: %w", err)
	}
	return nil
}

func (consumer *Consumer) Close() { consumer.client.CloseAllowingRebalance() }

var _ ports.EventConsumer = (*Consumer)(nil)
