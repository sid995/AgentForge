package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DeliveryDeadLetteredType = "event-delivery.dead-lettered.v1"
	AgentRunDLQTopic         = "agentforge.agent-run.lifecycle.dlq.v1"
)

// DeadLetterPayload preserves a valid original or a non-sensitive malformed fingerprint.
type DeadLetterPayload struct {
	OriginalEnvelope  json.RawMessage `json:"originalEnvelope,omitempty"`
	RawFingerprint    string          `json:"rawFingerprint,omitempty"`
	RawSize           int             `json:"rawSize,omitempty"`
	OriginalTopic     string          `json:"originalTopic"`
	OriginalPartition int32           `json:"originalPartition"`
	OriginalOffset    int64           `json:"originalOffset"`
	OriginalKey       string          `json:"originalKey,omitempty"`
	OriginalHeaders   []Header        `json:"originalHeaders,omitempty"`
	ConsumerIdentity  string          `json:"consumerIdentity"`
	FailureCode       string          `json:"failureCode"`
	FailureReason     string          `json:"failureReason"`
	Attempts          int             `json:"attempts"`
	FirstFailedAt     time.Time       `json:"firstFailedAt"`
	LastFailedAt      time.Time       `json:"lastFailedAt"`
	ReplayDisposition string          `json:"replayDisposition"`
}

// DeadLetterSource is bounded provider-neutral source metadata.
type DeadLetterSource struct {
	Envelope  Envelope
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Headers   []Header
	Value     []byte
}

// NewDeadLetter creates a canonical DLQ event for a valid original envelope.
func NewDeadLetter(original DeadLetterSource, consumerIdentity, code, reason string, attempts int, firstFailedAt, now time.Time) (OutboxEvent, error) {
	if err := ValidateEnvelope(original.Envelope); err != nil {
		return OutboxEvent{}, err
	}
	payload := DeadLetterPayload{OriginalEnvelope: json.RawMessage(original.Value), OriginalTopic: original.Topic, OriginalPartition: original.Partition, OriginalOffset: original.Offset, OriginalKey: string(original.Key), OriginalHeaders: safeOriginalHeaders(original.Headers), ConsumerIdentity: strings.TrimSpace(consumerIdentity), FailureCode: strings.TrimSpace(code), FailureReason: strings.TrimSpace(reason), Attempts: attempts, FirstFailedAt: firstFailedAt.UTC(), LastFailedAt: now.UTC(), ReplayDisposition: "QUARANTINED"}
	failureID, err := uuid.NewV7()
	if err != nil {
		return OutboxEvent{}, err
	}
	event, err := newEvent(DeliveryDeadLetteredType, 1, now, "event-consumer", original.Envelope.TenantID, original.Envelope.ProjectID, original.Envelope.RunID, "EventDeliveryFailure", failureID, 1, original.Envelope.CorrelationID, original.Envelope.EventID.String(), payload)
	if err != nil {
		return OutboxEvent{}, err
	}
	event.Topic, event.PartitionKey = AgentRunDLQTopic, original.Envelope.AggregateID.String()
	return event, nil
}

// NewMalformedDeadLetter never embeds malformed bytes; the caller supplies a trusted quarantine tenant.
func NewMalformedDeadLetter(original DeadLetterSource, quarantineTenantID uuid.UUID, consumerIdentity, code, reason string, attempts int, firstFailedAt, now time.Time) (OutboxEvent, error) {
	fingerprint := sha256.Sum256(original.Value)
	fingerprintText := "sha256:" + hex.EncodeToString(fingerprint[:])
	payload := DeadLetterPayload{RawFingerprint: fingerprintText, RawSize: len(original.Value), OriginalTopic: original.Topic, OriginalPartition: original.Partition, OriginalOffset: original.Offset, ConsumerIdentity: strings.TrimSpace(consumerIdentity), FailureCode: strings.TrimSpace(code), FailureReason: strings.TrimSpace(reason), Attempts: attempts, FirstFailedAt: firstFailedAt.UTC(), LastFailedAt: now.UTC(), ReplayDisposition: "QUARANTINED"}
	failureID, err := uuid.NewV7()
	if err != nil {
		return OutboxEvent{}, err
	}
	event, err := newEvent(DeliveryDeadLetteredType, 1, now, "event-consumer", quarantineTenantID, nil, nil, "EventDeliveryFailure", failureID, 1, fingerprintText, fingerprintText, payload)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("create malformed dead letter: %w", err)
	}
	event.Topic, event.PartitionKey = AgentRunDLQTopic, failureID.String()
	return event, nil
}

func safeOriginalHeaders(headers []Header) []Header {
	allowed := map[string]bool{"event-id": true, "event-type": true, "schema-version": true, "tenant-id": true, "correlation-id": true, "causation-id": true, "request-id": true, "traceparent": true, "tracestate": true, "delivery-attempt": true, "retry-not-before": true}
	safe := make([]Header, 0, len(headers))
	for _, header := range headers {
		if allowed[header.Key] {
			safe = append(safe, header)
		}
	}
	return safe
}
