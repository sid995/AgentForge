package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

const (
	AgentRunRequestedType = "agent-run.requested.v1"
	maxHeaderValueLength  = 512
	maxHeaderCount        = 32
	maxHeaderBytes        = 4096
)

var traceparentPattern = regexp.MustCompile(`^[\da-f]{2}-[\da-f]{32}-[\da-f]{16}-[\da-f]{2}$`)

// Header is provider-independent Kafka routing and correlation metadata.
type Header struct {
	Key   string
	Value string
}

// DecodeEnvelope strictly decodes and validates one supported event envelope.
func DecodeEnvelope(serialized []byte) (Envelope, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(serialized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode event envelope: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return Envelope{}, err
	}
	if err := ValidateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// ValidateEnvelope enforces identity, version, correlation, and typed payload rules.
func ValidateEnvelope(envelope Envelope) error {
	if envelope.EventID == uuid.Nil || envelope.EventID.Version() != 7 || !eventTypePattern.MatchString(envelope.EventType) {
		return fmt.Errorf("event ID or type is invalid")
	}
	major, err := eventMajor(envelope.EventType)
	if err != nil || envelope.SchemaVersion != major {
		return fmt.Errorf("event schema version does not match event type")
	}
	_, timezoneOffset := envelope.OccurredAt.Zone()
	if envelope.OccurredAt.IsZero() || timezoneOffset != 0 || strings.TrimSpace(envelope.Producer) == "" || len(envelope.Producer) > 128 {
		return fmt.Errorf("event occurrence and producer are required")
	}
	if !validV7(envelope.TenantID) || !validOptionalV7(envelope.ProjectID) || !validOptionalV7(envelope.RunID) || !validV7(envelope.AggregateID) || strings.TrimSpace(envelope.AggregateType) == "" || envelope.AggregateVersion < 1 {
		return fmt.Errorf("event ownership or aggregate metadata is invalid")
	}
	if strings.TrimSpace(envelope.CorrelationID) == "" || len(envelope.CorrelationID) > 255 || strings.TrimSpace(envelope.CausationID) == "" || len(envelope.CausationID) > 255 || len(envelope.RequestID) > 128 {
		return fmt.Errorf("event correlation metadata is invalid")
	}
	if envelope.Traceparent != "" && (!traceparentPattern.MatchString(envelope.Traceparent) || envelope.Traceparent[3:35] == strings.Repeat("0", 32) || envelope.Traceparent[36:52] == strings.Repeat("0", 16)) {
		return fmt.Errorf("event traceparent is invalid")
	}
	if len(envelope.Tracestate) > maxHeaderValueLength {
		return fmt.Errorf("event tracestate is too long")
	}
	var payloadObject map[string]json.RawMessage
	if len(envelope.Payload) == 0 || json.Unmarshal(envelope.Payload, &payloadObject) != nil || payloadObject == nil {
		return fmt.Errorf("event payload must be a JSON object")
	}
	switch envelope.EventType {
	case AgentRunRequestedType:
		return validateAgentRunRequested(envelope)
	default:
		return fmt.Errorf("unsupported event type %q", envelope.EventType)
	}
}

// HeadersForEnvelope returns the bounded headers repeated from the body.
func HeadersForEnvelope(envelope Envelope) []Header {
	headers := []Header{
		{Key: "event-id", Value: envelope.EventID.String()},
		{Key: "event-type", Value: envelope.EventType},
		{Key: "schema-version", Value: strconv.Itoa(envelope.SchemaVersion)},
		{Key: "tenant-id", Value: envelope.TenantID.String()},
		{Key: "correlation-id", Value: envelope.CorrelationID},
		{Key: "causation-id", Value: envelope.CausationID},
	}
	for _, optional := range []Header{{Key: "request-id", Value: envelope.RequestID}, {Key: "traceparent", Value: envelope.Traceparent}, {Key: "tracestate", Value: envelope.Tracestate}} {
		if optional.Value != "" {
			headers = append(headers, optional)
		}
	}
	return headers
}

// ValidateHeaders rejects missing, duplicate, oversized, or body-mismatched headers.
func ValidateHeaders(envelope Envelope, headers []Header) error {
	if len(headers) > maxHeaderCount {
		return fmt.Errorf("kafka record has too many headers")
	}
	actual := make(map[string]string, len(headers))
	headerBytes := 0
	repeated := map[string]bool{"event-id": true, "event-type": true, "schema-version": true, "tenant-id": true, "correlation-id": true, "causation-id": true, "request-id": true, "traceparent": true, "tracestate": true}
	expected := make(map[string]string)
	for _, header := range HeadersForEnvelope(envelope) {
		expected[header.Key] = header.Value
	}
	for _, header := range headers {
		headerBytes += len(header.Key) + len(header.Value)
		if header.Key == "" || len(header.Key) > 64 || len(header.Value) > maxHeaderValueLength || headerBytes > maxHeaderBytes {
			return fmt.Errorf("kafka header %q is too long", header.Key)
		}
		if _, exists := actual[header.Key]; exists {
			return fmt.Errorf("kafka header %q is duplicated", header.Key)
		}
		actual[header.Key] = header.Value
		if repeated[header.Key] && expected[header.Key] != header.Value {
			return fmt.Errorf("kafka header %q does not match event body", header.Key)
		}
	}
	for key, value := range expected {
		if actual[key] != value {
			return fmt.Errorf("kafka header %q does not match event body", key)
		}
	}
	return nil
}

func validateAgentRunRequested(envelope Envelope) error {
	if envelope.AggregateType != "AgentRun" || envelope.RunID == nil || envelope.ProjectID == nil || *envelope.RunID != envelope.AggregateID {
		return fmt.Errorf("run event aggregate metadata is invalid")
	}
	var payload AgentRunRequestedPayload
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode run-request payload: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return err
	}
	if payload.ProjectID != *envelope.ProjectID || payload.RunID != *envelope.RunID || payload.Status != domain.AgentRunQueued || payload.AttemptNumber != 1 {
		return fmt.Errorf("run-request payload identity or state is invalid")
	}
	promptReference, promptErr := url.Parse(payload.PromptReference)
	if strings.TrimSpace(payload.Runtime) == "" || len(payload.Runtime) > 120 || payload.CPUMillis < 1 || payload.CPUMillis > 128000 || payload.MemoryMiB < 1 || payload.MemoryMiB > 524288 || payload.TimeoutSeconds < 1 || payload.TimeoutSeconds > 86400 || payload.MaxAttempts < 1 || payload.MaxAttempts > 10 || strings.TrimSpace(payload.PromptReference) == "" || len(payload.PromptReference) > 2048 || promptErr != nil || promptReference.Scheme == "" || !strings.Contains(payload.PromptReference, "://") {
		return fmt.Errorf("run-request payload constraints are invalid")
	}
	return nil
}

func eventMajor(eventType string) (int, error) {
	index := strings.LastIndex(eventType, ".v")
	if index < 0 {
		return 0, fmt.Errorf("event type has no major version")
	}
	return strconv.Atoi(eventType[index+2:])
}

func validV7(id uuid.UUID) bool { return id != uuid.Nil && id.Version() == 7 }

func validOptionalV7(id *uuid.UUID) bool { return id == nil || validV7(*id) }

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("event JSON contains trailing data")
	}
	return nil
}
