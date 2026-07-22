package postgres

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// ConsumerMetrics receives bounded event-processing outcomes.
type ConsumerMetrics interface {
	Processed(string)
	Duplicate(string)
	Failed(string)
}

type noConsumerMetrics struct{}

func (noConsumerMetrics) Processed(string) {}
func (noConsumerMetrics) Duplicate(string) {}
func (noConsumerMetrics) Failed(string)    {}

// ProcessedEventRepository exposes the explicit marker/business transaction.
type ProcessedEventRepository struct {
	database *database.Pool
	metrics  ConsumerMetrics
}

func NewProcessedEventRepository(pool *database.Pool, metrics ConsumerMetrics) *ProcessedEventRepository {
	if metrics == nil {
		metrics = noConsumerMetrics{}
	}
	return &ProcessedEventRepository{database: pool, metrics: metrics}
}

// Process inserts the consumer marker and business effect in one tenant transaction.
// The caller acknowledges the broker only after this method returns successfully.
func (repository *ProcessedEventRepository) Process(ctx context.Context, consumerIdentity string, event ports.ReceivedEvent, processedAt time.Time, businessEffect func(context.Context, *database.TenantTx) error) (bool, error) {
	consumerIdentity = strings.TrimSpace(consumerIdentity)
	if consumerIdentity == "" || len(consumerIdentity) > 160 || businessEffect == nil {
		return false, fmt.Errorf("consumer identity and business effect are required")
	}
	if err := validateReceivedEvent(event); err != nil {
		return false, err
	}
	tx, err := repository.database.BeginTenant(ctx, event.Envelope.TenantID)
	if err != nil {
		return false, fmt.Errorf("begin processed-event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		insert into processed_events (consumer_identity, event_id, tenant_id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version, source_topic, source_partition, source_offset, processed_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		on conflict (consumer_identity, event_id) do nothing`, consumerIdentity, event.Envelope.EventID, event.Envelope.TenantID, event.Envelope.EventType, event.Envelope.SchemaVersion, event.Envelope.AggregateType, event.Envelope.AggregateID, event.Envelope.AggregateVersion, event.Topic, event.Partition, event.Offset, processedAt.UTC())
	if err != nil {
		repository.metrics.Failed("MARKER_INSERT")
		return false, fmt.Errorf("insert processed-event marker: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read processed-event marker result: %w", err)
	}
	if inserted == 0 {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate event transaction: %w", err)
		}
		repository.metrics.Duplicate(event.Envelope.EventType)
		return true, nil
	}
	if err := businessEffect(ctx, tx); err != nil {
		repository.metrics.Failed("BUSINESS_EFFECT")
		return false, fmt.Errorf("apply event business effect: %w", err)
	}
	if err := tx.Commit(); err != nil {
		repository.metrics.Failed("COMMIT")
		return false, fmt.Errorf("commit processed event: %w", err)
	}
	repository.metrics.Processed(event.Envelope.EventType)
	return false, nil
}

func validateReceivedEvent(event ports.ReceivedEvent) error {
	if event.Envelope.EventID.Version() != 7 || event.Envelope.TenantID.Version() != 7 || event.Topic == "" || event.Partition < 0 || event.Offset < 0 {
		return fmt.Errorf("validated event source metadata is required")
	}
	decoded, err := events.DecodeEnvelope(event.Value)
	if err != nil || decoded.EventID != event.Envelope.EventID {
		return fmt.Errorf("validated event envelope bytes are required")
	}
	if err := events.ValidateHeaders(decoded, event.Headers); err != nil {
		return err
	}
	expectedKey, err := events.ExpectedPartitionKey(decoded)
	if err != nil || !events.TopicAccepts(decoded.EventType, event.Topic) || !bytes.Equal(event.Key, []byte(expectedKey)) {
		return fmt.Errorf("validated event topic and partition key are required")
	}
	return nil
}
