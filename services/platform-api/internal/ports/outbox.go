package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/events"
)

// ClaimedOutboxEvent is an event durably leased by one relay instance.
type ClaimedOutboxEvent struct {
	events.OutboxEvent
	PublicationAttempts int
	ClaimedBy           string
	ClaimExpiresAt      time.Time
}

// PublicationResult identifies the acknowledged broker record.
type PublicationResult struct {
	Partition int
	Offset    int64
}

// OutboxRepository is the explicit durable relay boundary.
type OutboxRepository interface {
	Claim(context.Context, string, time.Time, time.Duration, int) ([]ClaimedOutboxEvent, error)
	MarkPublished(context.Context, uuid.UUID, string, time.Time, PublicationResult) error
	MarkFailed(context.Context, uuid.UUID, string, time.Time, string, string, bool) error
	CleanupPublished(context.Context, time.Time, int) (int64, error)
}

// EventPublisher is implemented by the Phase 4.3 Kafka adapter.
type EventPublisher interface {
	Publish(context.Context, events.OutboxEvent) (PublicationResult, error)
	Close() error
}
