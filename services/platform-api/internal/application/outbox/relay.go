// Package outbox coordinates durable publication without depending on Kafka types.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// Metrics receives bounded relay observations.
type Metrics interface {
	Claimed(int)
	Published(string, time.Duration)
	Failed(string, bool)
}

type noMetrics struct{}

func (noMetrics) Claimed(int)                     {}
func (noMetrics) Published(string, time.Duration) {}
func (noMetrics) Failed(string, bool)             {}

// PermanentError marks a publication failure that cannot be retried unchanged.
type PermanentError interface {
	error
	Permanent() bool
}

// SafeError exposes a bounded, secret-free message suitable for durable storage.
type SafeError interface {
	error
	SafeMessage() string
}

// Relay publishes bounded, durably claimed batches.
type Relay struct {
	repository     ports.OutboxRepository
	publisher      ports.EventPublisher
	metrics        Metrics
	owner          string
	now            func() time.Time
	lease          time.Duration
	publishTimeout time.Duration
	batchSize      int
	pollInterval   time.Duration
}

func NewRelay(repository ports.OutboxRepository, publisher ports.EventPublisher, metrics Metrics, owner string, now func() time.Time) (*Relay, error) {
	if repository == nil || publisher == nil || owner == "" {
		return nil, fmt.Errorf("outbox repository, publisher, and owner are required")
	}
	if metrics == nil {
		metrics = noMetrics{}
	}
	if now == nil {
		now = time.Now
	}
	return &Relay{repository: repository, publisher: publisher, metrics: metrics, owner: owner, now: now, lease: 30 * time.Second, publishTimeout: 10 * time.Second, batchSize: 100, pollInterval: time.Second}, nil
}

// RunOnce claims and processes one batch.
func (relay *Relay) RunOnce(ctx context.Context) (int, error) {
	claimed, err := relay.repository.Claim(ctx, relay.owner, relay.now(), relay.lease, relay.batchSize)
	if err != nil {
		return 0, err
	}
	relay.metrics.Claimed(len(claimed))
	var failures []error
	for _, event := range claimed {
		started := relay.now()
		publishContext, cancel := context.WithTimeout(ctx, relay.publishTimeout)
		result, publishErr := relay.publisher.Publish(publishContext, event.OutboxEvent)
		cancel()
		if publishErr == nil {
			// Mark only after the broker accepts the record. If this write fails, the
			// lease expires and at-least-once publication retries the same event.
			if err := relay.repository.MarkPublished(ctx, event.Envelope.EventID, relay.owner, relay.now(), result); err != nil {
				failures = append(failures, err)
				continue
			}
			relay.metrics.Published(event.Envelope.EventType, relay.now().Sub(started))
			continue
		}
		permanent := false
		var classified PermanentError
		if errors.As(publishErr, &classified) {
			permanent = classified.Permanent()
		}
		if err := relay.repository.MarkFailed(ctx, event.Envelope.EventID, relay.owner, relay.now(), failureCategory(permanent), safeFailureReason(publishErr), permanent); err != nil {
			failures = append(failures, err)
			continue
		}
		relay.metrics.Failed(event.Envelope.EventType, permanent)
	}
	return len(claimed), errors.Join(failures...)
}

func safeFailureReason(err error) string {
	var safe SafeError
	if errors.As(err, &safe) {
		if message := strings.TrimSpace(safe.SafeMessage()); message != "" {
			return message
		}
	}
	return "event publication failed"
}

// Run polls until cancellation, then closes the publisher after in-flight work returns.
func (relay *Relay) Run(ctx context.Context) error {
	defer func() { _ = relay.publisher.Close() }()
	ticker := time.NewTicker(relay.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := relay.RunOnce(ctx); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func failureCategory(permanent bool) string {
	if permanent {
		return "PERMANENT"
	}
	return "TRANSIENT"
}
