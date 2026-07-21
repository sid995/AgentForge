package kafka

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
)

type fakeProducer struct {
	record *kgo.Record
	err    error
	closed bool
}

func (producer *fakeProducer) ProduceSync(_ context.Context, records ...*kgo.Record) kgo.ProduceResults {
	producer.record = records[0]
	producer.record.Partition = 2
	producer.record.Offset = 42
	return kgo.ProduceResults{{Record: producer.record, Err: producer.err}}
}
func (producer *fakeProducer) Close() { producer.closed = true }

type fakeGroup struct {
	committed *kgo.Record
	closed    bool
}

func (*fakeGroup) PollRecords(context.Context, int) kgo.Fetches { return nil }
func (group *fakeGroup) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	group.committed = records[0]
	return nil
}
func (group *fakeGroup) CloseAllowingRebalance() { group.closed = true }

func TestProducerMapsValidatedEnvelopeAndHeaders(t *testing.T) {
	event := fixtureEvent(t)
	client := &fakeProducer{}
	producer := &Producer{client: client, maxMessageBytes: 1 << 20}
	result, err := producer.Publish(context.Background(), event)
	if err != nil || result.Partition != 2 || result.Offset != 42 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if client.record.Topic != events.AgentRunLifecycleTopic || string(client.record.Key) != event.PartitionKey || len(client.record.Headers) < 6 {
		t.Fatalf("record=%#v", client.record)
	}
	if err := producer.Close(); err != nil || !client.closed {
		t.Fatalf("close err=%v closed=%t", err, client.closed)
	}
}

func TestProducerClassifiesInvalidAndBrokerFailures(t *testing.T) {
	event := fixtureEvent(t)
	producer := &Producer{client: &fakeProducer{}, maxMessageBytes: 10}
	if _, err := producer.Publish(context.Background(), event); !isPermanent(err) {
		t.Fatalf("oversized event error=%v", err)
	}
	for _, test := range []struct {
		err       error
		permanent bool
	}{
		{err: kerr.NetworkException},
		{err: kgo.ErrRecordRetries},
		{err: kgo.ErrRecordTimeout},
		{err: kerr.TopicAuthorizationFailed, permanent: true},
		{err: kerr.UnknownTopicOrPartition, permanent: true},
		{err: errors.New("unclassified provider failure"), permanent: true},
	} {
		classified := classifyPublicationError(test.err)
		if isPermanent(classified) != test.permanent || classified.Error() == test.err.Error() {
			t.Fatalf("classified=%v permanent=%t", classified, isPermanent(classified))
		}
	}
}

func TestConsumerRejectsHeaderAndKeyMismatches(t *testing.T) {
	event := fixtureEvent(t)
	record := recordFor(event)
	consumer := &Consumer{maxMessageBytes: 1 << 20}
	received, err := consumer.decodeRecord(record)
	if err != nil || received.Envelope.EventID != event.Envelope.EventID {
		t.Fatalf("received=%#v err=%v", received, err)
	}
	record.Key = []byte("wrong")
	if _, err := consumer.decodeRecord(record); !isPermanent(err) {
		t.Fatalf("key mismatch error=%v", err)
	}
	record = recordFor(event)
	record.Headers[0].Value = []byte("019b0000-0000-7000-8000-000000000000")
	if _, err := consumer.decodeRecord(record); !isPermanent(err) {
		t.Fatalf("header mismatch error=%v", err)
	}
}

func TestConsumerAcknowledgesExactSourceOffsetAndLeaderEpoch(t *testing.T) {
	group := &fakeGroup{}
	consumer := &Consumer{client: group, maxMessageBytes: 1 << 20}
	event := fixtureEvent(t)
	received := recordFor(event)
	received.Partition, received.LeaderEpoch, received.Offset = 2, 7, 42
	decoded, err := consumer.decodeRecord(received)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.Acknowledge(context.Background(), decoded); err != nil {
		t.Fatal(err)
	}
	consumer.Close()
	if group.committed.Partition != 2 || group.committed.LeaderEpoch != 7 || group.committed.Offset != 42 || !group.closed {
		t.Fatalf("committed=%#v closed=%t", group.committed, group.closed)
	}
}

func fixtureEvent(t *testing.T) events.OutboxEvent {
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
	return events.OutboxEvent{Envelope: envelope, Topic: events.AgentRunLifecycleTopic, PartitionKey: envelope.AggregateID.String(), Serialized: serialized, CreatedAt: envelope.OccurredAt}
}

func recordFor(event events.OutboxEvent) *kgo.Record {
	record := &kgo.Record{Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Serialized}
	for _, header := range events.HeadersForEnvelope(event.Envelope) {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: header.Key, Value: []byte(header.Value)})
	}
	return record
}

func isPermanent(err error) bool {
	var permanent interface{ Permanent() bool }
	return errors.As(err, &permanent) && permanent.Permanent()
}
