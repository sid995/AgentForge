package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// OutboxRepository owns cross-tenant relay operations through the isolated relay role.
type OutboxRepository struct{ database *database.Pool }

func NewOutboxRepository(pool *database.Pool) *OutboxRepository {
	return &OutboxRepository{database: pool}
}

type contextExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertOutboxEvent(ctx context.Context, tx contextExecer, event events.OutboxEvent) error {
	_, err := tx.ExecContext(ctx, `
		insert into outbox_events (event_id, tenant_id, project_id, run_id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version, topic, partition_key, envelope, next_attempt_at, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14)`,
		event.Envelope.EventID, event.Envelope.TenantID, event.Envelope.ProjectID, event.Envelope.RunID, event.Envelope.EventType, event.Envelope.SchemaVersion, event.Envelope.AggregateType, event.Envelope.AggregateID, event.Envelope.AggregateVersion, event.Topic, event.PartitionKey, event.Serialized, event.CreatedAt, event.CreatedAt)
	return err
}

func (repository *OutboxRepository) Claim(ctx context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]ports.ClaimedOutboxEvent, error) {
	if owner == "" || lease <= 0 || limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("outbox claim owner, lease, and bounded limit are required")
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Claim-and-lease is deliberately separate from broker publication: a worker
	// crash may duplicate delivery, but cannot lose a committed outbox event.
	rows, err := tx.QueryContext(ctx, `
		with claimable as (
			select event_id from outbox_events
			where disposition = 'PENDING' and published_at is null and next_attempt_at <= $1
			  and (claimed_by is null or claim_expires_at <= $1)
			order by next_attempt_at, created_at, event_id
			for update skip locked limit $2
		)
		update outbox_events event set claimed_by = $3, claim_expires_at = $4
		from claimable where event.event_id = claimable.event_id
		returning event.event_id, event.tenant_id, event.project_id, event.run_id, event.event_type, event.schema_version, event.aggregate_type, event.aggregate_id, event.aggregate_version, event.topic, event.partition_key, event.envelope, event.created_at, event.publication_attempts, event.claimed_by, event.claim_expires_at`, now.UTC(), limit, owner, now.UTC().Add(lease))
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	claimed := make([]ports.ClaimedOutboxEvent, 0, limit)
	for rows.Next() {
		event, scanErr := scanClaimedOutbox(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		claimed = append(claimed, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox claims: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit outbox claims: %w", err)
	}
	return claimed, nil
}

func (repository *OutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID, owner string, now time.Time, result ports.PublicationResult) error {
	res, err := repository.database.Raw().ExecContext(ctx, `
		update outbox_events set disposition = 'PUBLISHED', publication_attempts = publication_attempts + 1,
			published_at = $1, broker_partition = $2, broker_offset = $3, claimed_by = null, claim_expires_at = null,
			last_failure_category = null, last_failure_reason = null, last_failure_at = null
		where event_id = $4 and disposition = 'PENDING' and claimed_by = $5`, now.UTC(), result.Partition, result.Offset, eventID, owner)
	return requireOneOutboxRow(res, err, "mark outbox event published")
}

func (repository *OutboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, owner string, now time.Time, category, reason string, permanent bool) error {
	disposition := "PENDING"
	if permanent {
		disposition = "TERMINAL"
	}
	res, err := repository.database.Raw().ExecContext(ctx, `
		update outbox_events set disposition = $1, publication_attempts = publication_attempts + 1,
			next_attempt_at = case when $2 then $3::timestamptz else $3::timestamptz + (case publication_attempts
				when 0 then interval '1 second' when 1 then interval '5 seconds' when 2 then interval '30 seconds'
				when 3 then interval '2 minutes' else interval '10 minutes' end *
				(0.8 + mod(abs(hashtextextended(event_id::text, publication_attempts)::numeric), 41) / 100)) end,
			claimed_by = null, claim_expires_at = null, last_failure_category = $4, last_failure_reason = left($5, 1000), last_failure_at = $3
		where event_id = $6 and disposition = 'PENDING' and claimed_by = $7`, disposition, permanent, now.UTC(), category, reason, eventID, owner)
	return requireOneOutboxRow(res, err, "mark outbox event failed")
}

func (repository *OutboxRepository) CleanupPublished(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 || limit > 10000 {
		return 0, fmt.Errorf("cleanup limit must be between 1 and 10000")
	}
	res, err := repository.database.Raw().ExecContext(ctx, `
		delete from outbox_events where event_id in (
			select event_id from outbox_events where disposition = 'PUBLISHED' and published_at < $1
			order by published_at, event_id limit $2
		)`, before.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("cleanup published outbox events: %w", err)
	}
	return res.RowsAffected()
}

func scanClaimedOutbox(row interface{ Scan(...any) error }) (ports.ClaimedOutboxEvent, error) {
	var claimed ports.ClaimedOutboxEvent
	var serialized []byte
	err := row.Scan(&claimed.Envelope.EventID, &claimed.Envelope.TenantID, &claimed.Envelope.ProjectID, &claimed.Envelope.RunID, &claimed.Envelope.EventType, &claimed.Envelope.SchemaVersion, &claimed.Envelope.AggregateType, &claimed.Envelope.AggregateID, &claimed.Envelope.AggregateVersion, &claimed.Topic, &claimed.PartitionKey, &serialized, &claimed.CreatedAt, &claimed.PublicationAttempts, &claimed.ClaimedBy, &claimed.ClaimExpiresAt)
	if err != nil {
		return claimed, fmt.Errorf("scan claimed outbox event: %w", err)
	}
	if err := json.Unmarshal(serialized, &claimed.Envelope); err != nil {
		return claimed, fmt.Errorf("decode claimed outbox envelope: %w", err)
	}
	claimed.Serialized = serialized
	return claimed, nil
}

func requireOneOutboxRow(result sql.Result, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", operation, err)
	}
	if count != 1 {
		return fmt.Errorf("%s: claim is missing or expired", operation)
	}
	return nil
}

var _ ports.OutboxRepository = (*OutboxRepository)(nil)
