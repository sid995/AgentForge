// Package events defines provider-independent integration event contracts.
package events

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

const AgentRunLifecycleTopic = "agentforge.agent-run.lifecycle.v1"

var eventTypePattern = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+\.v[1-9][0-9]*$`)

// Envelope is the canonical, provider-independent event representation.
type Envelope struct {
	EventID          uuid.UUID       `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	TenantID         uuid.UUID       `json:"tenantId"`
	ProjectID        *uuid.UUID      `json:"projectId,omitempty"`
	RunID            *uuid.UUID      `json:"runId,omitempty"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      uuid.UUID       `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	RequestID        string          `json:"requestId,omitempty"`
	Traceparent      string          `json:"traceparent,omitempty"`
	Tracestate       string          `json:"tracestate,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	CausationID      string          `json:"causationId"`
	Payload          json.RawMessage `json:"payload"`
}

// OutboxEvent is a serialized event intent awaiting publication.
type OutboxEvent struct {
	Envelope     Envelope
	Topic        string
	PartitionKey string
	Headers      []Header
	Serialized   []byte
	CreatedAt    time.Time
}

// AgentRunRequestedPayload is an allowlisted payload with no raw prompt data.
type AgentRunRequestedPayload struct {
	ProjectID       uuid.UUID             `json:"projectId"`
	RunID           uuid.UUID             `json:"runId"`
	Status          domain.AgentRunStatus `json:"status"`
	AttemptNumber   int                   `json:"attemptNumber"`
	Runtime         string                `json:"runtime"`
	CPUMillis       int                   `json:"cpuMillis"`
	MemoryMiB       int                   `json:"memoryMiB"`
	TimeoutSeconds  int                   `json:"timeoutSeconds"`
	MaxAttempts     int                   `json:"maxAttempts"`
	PromptReference string                `json:"promptReference"`
}

type AgentRunScheduledPayload struct {
	ProjectID             uuid.UUID             `json:"projectId"`
	RunID                 uuid.UUID             `json:"runId"`
	Status                domain.AgentRunStatus `json:"status"`
	AttemptID             uuid.UUID             `json:"attemptId"`
	AttemptNumber         int                   `json:"attemptNumber"`
	ClusterID             string                `json:"clusterId"`
	ExecutionProfile      string                `json:"executionProfile"`
	CapacityReservationID uuid.UUID             `json:"capacityReservationId"`
	BudgetReservationID   uuid.UUID             `json:"budgetReservationId"`
	CPUMillis             int                   `json:"cpuMillis"`
	MemoryMiB             int                   `json:"memoryMiB"`
}

type AgentRunCapacityWaitPayload struct {
	ProjectID      uuid.UUID             `json:"projectId"`
	RunID          uuid.UUID             `json:"runId"`
	Status         domain.AgentRunStatus `json:"status"`
	AttemptNumber  int                   `json:"attemptNumber"`
	ReasonCode     string                `json:"reasonCode"`
	Reason         string                `json:"reason"`
	NextEligibleAt time.Time             `json:"nextEligibleAt"`
}

// newEvent constructs and serializes a typed event before its business transaction begins.
func newEvent(
	eventType string,
	schemaVersion int,
	occurredAt time.Time,
	producer string,
	tenantID uuid.UUID,
	projectID,
	runID *uuid.UUID,
	aggregateType string,
	aggregateID uuid.UUID,
	aggregateVersion int64,
	correlationID,
	causationID string,
	payload any,
) (OutboxEvent, error) {
	if !eventTypePattern.MatchString(eventType) || schemaVersion < 1 || tenantID == uuid.Nil || aggregateID == uuid.Nil || aggregateVersion < 1 {
		return OutboxEvent{}, fmt.Errorf("event identity and version are invalid")
	}
	producer = strings.TrimSpace(producer)
	correlationID = strings.TrimSpace(correlationID)
	causationID = strings.TrimSpace(causationID)

	if producer == "" || aggregateType == "" || correlationID == "" || causationID == "" {
		return OutboxEvent{}, fmt.Errorf("event producer, aggregate, correlation, and causation are required")
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("serialize event payload: %w", err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("generate event ID: %w", err)
	}
	envelope := Envelope{EventID: eventID, EventType: eventType, SchemaVersion: schemaVersion, OccurredAt: occurredAt.UTC(), Producer: producer, TenantID: tenantID, ProjectID: projectID, RunID: runID, AggregateType: aggregateType, AggregateID: aggregateID, AggregateVersion: aggregateVersion, CorrelationID: correlationID, CausationID: causationID, Payload: payloadJSON}
	if err := ValidateEnvelope(envelope); err != nil {
		return OutboxEvent{}, err
	}
	serialized, err := json.Marshal(envelope)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("serialize event envelope: %w", err)
	}
	return OutboxEvent{
		Envelope:     envelope,
		Topic:        AgentRunLifecycleTopic,
		PartitionKey: aggregateID.String(),
		Serialized:   serialized,
		CreatedAt:    occurredAt.UTC(),
	}, nil
}

// NewAgentRunRequested creates the event intent atomically stored with a new run.
func NewAgentRunRequested(run domain.AgentRun, attempt domain.AgentRunAttempt) (OutboxEvent, error) {
	payload := AgentRunRequestedPayload{ProjectID: run.ProjectID, RunID: run.ID, Status: run.Status, AttemptNumber: attempt.AttemptNumber, Runtime: run.Runtime, CPUMillis: run.CPUMillis, MemoryMiB: run.MemoryMiB, TimeoutSeconds: run.TimeoutSeconds, MaxAttempts: run.MaxAttempts, PromptReference: run.PromptReference}
	return newEvent(
		AgentRunRequestedType,
		1,
		run.CreatedAt,
		"platform-api",
		run.TenantID,
		&run.ProjectID,
		&run.ID,
		"AgentRun",
		run.ID,
		run.Version,
		run.IdempotencyKey,
		run.IdempotencyKey,
		payload,
	)
}

func NewAgentRunScheduled(run domain.AgentRun, attempt domain.AgentRunAttempt, capacityReservationID, budgetReservationID uuid.UUID, correlationID, causationID string) (OutboxEvent, error) {
	payload := AgentRunScheduledPayload{ProjectID: run.ProjectID, RunID: run.ID, Status: run.Status, AttemptID: attempt.ID, AttemptNumber: attempt.AttemptNumber, ClusterID: attempt.SelectedCluster, ExecutionProfile: attempt.ExecutionProfile, CapacityReservationID: capacityReservationID, BudgetReservationID: budgetReservationID, CPUMillis: run.CPUMillis, MemoryMiB: run.MemoryMiB}
	return newEvent(AgentRunScheduledType, 1, run.UpdatedAt, "scheduler", run.TenantID, &run.ProjectID, &run.ID, "AgentRun", run.ID, run.Version, correlationID, causationID, payload)
}

func NewAgentRunCapacityWait(run domain.AgentRun, reasonCode, reason string, nextEligibleAt time.Time, correlationID, causationID string) (OutboxEvent, error) {
	payload := AgentRunCapacityWaitPayload{ProjectID: run.ProjectID, RunID: run.ID, Status: run.Status, AttemptNumber: run.AttemptCount, ReasonCode: reasonCode, Reason: reason, NextEligibleAt: nextEligibleAt.UTC()}
	return newEvent(AgentRunCapacityWaitType, 1, run.UpdatedAt, "scheduler", run.TenantID, &run.ProjectID, &run.ID, "AgentRun", run.ID, run.Version, correlationID, causationID, payload)
}
