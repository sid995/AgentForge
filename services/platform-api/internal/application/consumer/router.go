// Package consumer defines explicit post-transaction failure routing.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

const (
	RoutedRetry = "RETRY"
	RoutedDLQ   = "DLQ"
)

var retryTopics = []string{"agentforge.agent-run.lifecycle.retry.1m.v1", "agentforge.agent-run.lifecycle.retry.5m.v1", "agentforge.agent-run.lifecycle.retry.30m.v1"}
var retryDelays = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

// Metrics receives bounded retry and poison-message outcomes.
type Metrics interface {
	Retried(string, int)
	DeadLettered(string)
	RoutingFailed(string)
}

type noMetrics struct{}

func (noMetrics) Retried(string, int)  {}
func (noMetrics) DeadLettered(string)  {}
func (noMetrics) RoutingFailed(string) {}

// Failure is an explicitly classified, safely reportable processing error.
type Failure struct {
	cause     error
	code      string
	reason    string
	permanent bool
}

func (failure *Failure) Error() string       { return failure.reason }
func (failure *Failure) Unwrap() error       { return failure.cause }
func (failure *Failure) Permanent() bool     { return failure.permanent }
func (failure *Failure) SafeMessage() string { return failure.reason }
func (failure *Failure) Code() string        { return failure.code }

func NewTransientFailure(code, reason string, cause error) error {
	return newFailure(code, reason, cause, false)
}

func NewPermanentFailure(code, reason string, cause error) error {
	return newFailure(code, reason, cause, true)
}

func newFailure(code, reason string, cause error, permanent bool) error {
	code, reason = strings.TrimSpace(code), strings.TrimSpace(reason)
	if code == "" || len(code) > 80 || reason == "" || len(reason) > 500 {
		return &Failure{cause: cause, code: "INVALID_FAILURE", reason: "event processing failed", permanent: true}
	}
	return &Failure{cause: cause, code: code, reason: reason, permanent: permanent}
}

// Router publishes a retry or DLQ record before the caller acknowledges the source offset.
type Router struct {
	publisher          ports.EventPublisher
	metrics            Metrics
	consumerIdentity   string
	quarantineTenantID uuid.UUID
}

func NewRouter(publisher ports.EventPublisher, metrics Metrics, consumerIdentity string, quarantineTenantID uuid.UUID) (*Router, error) {
	consumerIdentity = strings.TrimSpace(consumerIdentity)
	if publisher == nil || consumerIdentity == "" || len(consumerIdentity) > 160 || quarantineTenantID.Version() != 7 {
		return nil, fmt.Errorf("publisher, consumer identity, and quarantine tenant are required")
	}
	if metrics == nil {
		metrics = noMetrics{}
	}
	return &Router{publisher: publisher, metrics: metrics, consumerIdentity: consumerIdentity, quarantineTenantID: quarantineTenantID}, nil
}

func (router *Router) Route(ctx context.Context, source ports.ReceivedEvent, processingError error, attempt int, firstFailedAt, now time.Time) (string, error) {
	if processingError == nil || attempt < 1 || firstFailedAt.IsZero() || now.Before(firstFailedAt) {
		return "", fmt.Errorf("processing failure metadata is invalid")
	}
	if source.Envelope.EventID != uuid.Nil {
		if expectedAttempt, err := expectedFailureAttempt(source); err != nil || expectedAttempt != attempt {
			return "", fmt.Errorf("processing attempt does not match source delivery metadata")
		}
	}
	permanent, code, reason := classify(processingError)
	if !permanent && attempt <= len(retryTopics) && events.ValidateEnvelope(source.Envelope) == nil {
		eventHeaders := []events.Header{
			{
				Key:   "delivery-attempt",
				Value: strconv.Itoa(attempt),
			},
			{
				Key:   "retry-not-before",
				Value: now.Add(retryDelays[attempt-1]).UTC().Format(time.RFC3339Nano),
			},
			{
				Key:   "original-topic",
				Value: source.Topic,
			},
			{
				Key:   "original-partition",
				Value: strconv.Itoa(int(source.Partition)),
			},
			{
				Key:   "original-offset",
				Value: strconv.FormatInt(source.Offset, 10),
			},
		}

		retry := events.OutboxEvent{
			Envelope:     source.Envelope,
			Topic:        retryTopics[attempt-1],
			PartitionKey: source.Envelope.AggregateID.String(),
			Serialized:   source.Value,
			CreatedAt:    source.Envelope.OccurredAt,
			Headers:      eventHeaders,
		}
		if _, err := router.publisher.Publish(ctx, retry); err != nil {
			router.metrics.RoutingFailed("RETRY_PUBLISH")
			return "", fmt.Errorf("publish retry event: %w", err)
		}
		router.metrics.Retried(source.Envelope.EventType, attempt)
		return RoutedRetry, nil
	}
	dlqSource := events.DeadLetterSource{Envelope: source.Envelope, Topic: source.Topic, Partition: source.Partition, Offset: source.Offset, Key: source.Key, Headers: source.Headers, Value: source.Value}

	var deadLetter events.OutboxEvent
	var err error

	if events.ValidateEnvelope(source.Envelope) == nil {
		deadLetter, err = events.NewDeadLetter(dlqSource, router.consumerIdentity, code, reason, attempt, firstFailedAt, now)
	} else {
		deadLetter, err = events.NewMalformedDeadLetter(dlqSource, router.quarantineTenantID, router.consumerIdentity, code, reason, attempt, firstFailedAt, now)
	}
	if err != nil {
		router.metrics.RoutingFailed("DLQ_BUILD")
		return "", err
	}
	deadLetter.Headers = []events.Header{{Key: "delivery-attempt", Value: strconv.Itoa(attempt)}}
	if _, err := router.publisher.Publish(ctx, deadLetter); err != nil {
		router.metrics.RoutingFailed("DLQ_PUBLISH")
		return "", fmt.Errorf("publish dead-letter event: %w", err)
	}
	router.metrics.DeadLettered(code)
	return RoutedDLQ, nil
}

// RetryNotBefore returns the explicit delayed-delivery boundary on a retry record.
func RetryNotBefore(event ports.ReceivedEvent) (time.Time, error) {
	if event.Topic != retryTopics[0] && event.Topic != retryTopics[1] && event.Topic != retryTopics[2] {
		return time.Time{}, fmt.Errorf("retry-not-before is only valid on retry topics")
	}
	value := ""
	for _, header := range event.Headers {
		if header.Key == "retry-not-before" {
			if value != "" {
				return time.Time{}, fmt.Errorf("retry-not-before header is duplicated")
			}
			value = header.Value
		}
	}
	if value == "" {
		return time.Time{}, fmt.Errorf("retry-not-before header is required")
	}
	notBefore, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("retry-not-before header is invalid")
	}
	return notBefore.UTC(), nil
}

func expectedFailureAttempt(event ports.ReceivedEvent) (int, error) {
	if event.Topic == events.AgentRunLifecycleTopic {
		return 1, nil
	}
	for index, topic := range retryTopics {
		if event.Topic != topic {
			continue
		}
		previous := index + 1
		matches := 0
		for _, header := range event.Headers {
			if header.Key == "delivery-attempt" {
				value, err := strconv.Atoi(header.Value)
				if err != nil || value != previous {
					return 0, fmt.Errorf("delivery-attempt header is invalid")
				}
				matches++
			}
		}
		if matches != 1 {
			return 0, fmt.Errorf("delivery-attempt header is required exactly once")
		}
		return previous + 1, nil
	}
	return 0, fmt.Errorf("source topic is not routable")
}

func classify(err error) (bool, string, string) {
	permanent := false
	var permanentError interface{ Permanent() bool }
	if errors.As(err, &permanentError) {
		permanent = permanentError.Permanent()
	}
	code := "TRANSIENT_PROCESSING"
	if permanent {
		code = "PERMANENT_PROCESSING"
	}
	var coded interface{ Code() string }
	if errors.As(err, &coded) && strings.TrimSpace(coded.Code()) != "" {
		code = coded.Code()
	}
	reason := "event processing failed"
	var safe interface{ SafeMessage() string }
	if errors.As(err, &safe) && strings.TrimSpace(safe.SafeMessage()) != "" {
		reason = safe.SafeMessage()
	}
	return permanent, code, reason
}
