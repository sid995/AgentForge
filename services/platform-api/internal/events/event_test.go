package events

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

func TestAgentRunRequestedEnvelopeContainsOnlySafeRunMetadata(t *testing.T) {
	now := time.Date(2026, 7, 21, 15, 0, 0, 0, time.UTC)
	run, err := domain.NewAgentRun(domain.NewAgentRunInput{TenantID: uuid.Must(uuid.NewV7()), ProjectID: uuid.Must(uuid.NewV7()), PromptReference: "vault://prompts/safe-reference", Runtime: "python-3.12", CPUMillis: 1000, MemoryMiB: 2048, TimeoutSeconds: 1800, MaxAttempts: 2, IdempotencyKey: "command-1", RequestHash: "sha256:request", CreatedBy: "developer"}, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	attempt, _ := domain.NewAgentRunAttempt(run.TenantID, run.ID, 1, now)
	event, err := NewAgentRunRequested(run, attempt)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	serialized := string(event.Serialized)
	if event.Envelope.EventID.Version() != 7 || event.Topic != AgentRunLifecycleTopic || event.PartitionKey != run.ID.String() || !strings.Contains(serialized, "vault://prompts/safe-reference") || strings.Contains(serialized, "requestHash") {
		t.Fatalf("unsafe or invalid event: %s", serialized)
	}
}

func TestNewRejectsSerializationFailure(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	if _, err := newEvent("agent-run.requested.v1", 1, time.Now(), "platform-api", id, nil, nil, "AgentRun", id, 1, "correlation", "cause", make(chan int)); err == nil {
		t.Fatal("newEvent() accepted an unserializable payload")
	}
}
