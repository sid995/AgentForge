package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	handoffapp "github.com/sid995/agentforge/services/platform-api/internal/application/handoff"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// HandoffRepository owns cluster authorization and durable processed markers
// for the Scheduler-to-Kubernetes projection.
type HandoffRepository struct {
	database  *database.Pool
	processed *ProcessedEventRepository
}

func NewHandoffRepository(pool *database.Pool) *HandoffRepository {
	return &HandoffRepository{database: pool, processed: NewProcessedEventRepository(pool, nil)}
}

func (repository *HandoffRepository) Authorize(ctx context.Context, clusterID string, tenantID uuid.UUID) error {
	var authorized bool
	err := repository.database.Raw().QueryRowContext(ctx, `
		select not cluster.tenant_restricted or exists (
			select 1 from cluster_tenant_allowlist allowed
			where allowed.cluster_id=cluster.id and allowed.tenant_id=$2
		)
		from clusters cluster where cluster.id=$1`, clusterID, tenantID).Scan(&authorized)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !authorized {
		return handoffapp.ErrUnauthorizedCluster
	}
	if err != nil {
		return fmt.Errorf("authorize handoff cluster: %w", err)
	}
	return nil
}

func (repository *HandoffRepository) Mark(ctx context.Context, identity string, event ports.ReceivedEvent, at time.Time) (bool, error) {
	return repository.processed.Process(ctx, identity, event, at, func(context.Context, *database.TenantTx) error { return nil })
}

func (repository *HandoffRepository) LoadIntent(ctx context.Context, event ports.ReceivedEvent, payload events.AgentRunScheduledPayload) (domain.AgentRunDesiredState, error) {
	var serialized []byte
	err := repository.database.Raw().QueryRowContext(ctx, `
		select desired_state from agentrun_handoff_intents
		where event_id=$1 and tenant_id=$2 and run_id=$3 and attempt_id=$4
		  and attempt_number=$5 and cluster_id=$6`,
		event.Envelope.EventID, event.Envelope.TenantID, payload.RunID, payload.AttemptID,
		payload.AttemptNumber, payload.ClusterID).Scan(&serialized)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AgentRunDesiredState{}, handoffapp.ErrIntentMissing
	}
	if err != nil {
		return domain.AgentRunDesiredState{}, fmt.Errorf("load AgentRun handoff intent: %w", err)
	}
	var desired domain.AgentRunDesiredState
	decoder := json.NewDecoder(bytes.NewReader(serialized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&desired); err != nil ||
		decoder.Decode(&struct{}{}) != io.EOF ||
		desired.Validate(payload.AttemptNumber) != nil ||
		desired.ExecutionProfile != payload.ExecutionProfile ||
		desired.CPUMillis != payload.CPUMillis || desired.MemoryMiB != payload.MemoryMiB {
		return domain.AgentRunDesiredState{}, handoffapp.ErrIntentInvalid
	}
	return desired, nil
}

func (repository *HandoffRepository) Seen(ctx context.Context, identity string, eventID, tenantID uuid.UUID) (bool, error) {
	var seen bool
	if err := repository.database.Raw().QueryRowContext(ctx, `select exists(select 1 from processed_events where consumer_identity=$1 and event_id=$2 and tenant_id=$3)`, identity, eventID, tenantID).Scan(&seen); err != nil {
		return false, fmt.Errorf("read processed-event marker: %w", err)
	}
	return seen, nil
}

var _ handoffapp.ClusterAuthorizer = (*HandoffRepository)(nil)
var _ handoffapp.IntentLoader = (*HandoffRepository)(nil)
var _ handoffapp.ProcessedMarker = (*HandoffRepository)(nil)
