package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type fakeEligibility struct {
	decision domain.EligibilityDecision
	err      error
}

func (repository fakeEligibility) Evaluate(context.Context, ports.ClaimedRun, time.Time) (domain.EligibilityDecision, error) {
	return repository.decision, repository.err
}

type eligibilityMetrics struct{ outcome, code string }

func (metrics *eligibilityMetrics) Decided(outcome, code string) {
	metrics.outcome, metrics.code = outcome, code
}

func TestEligibilityServiceEmitsBoundedDecisionTelemetry(t *testing.T) {
	metrics := &eligibilityMetrics{}
	var output bytes.Buffer
	decision := domain.EligibilityDecision{Outcome: domain.EligibilityDefer, Code: domain.EligibilityCodeCPU, Explanation: "tenant CPU limit has insufficient headroom"}
	service := NewEligibilityService(fakeEligibility{decision: decision}, metrics, slog.New(slog.NewJSONHandler(&output, nil)))
	run := ports.ClaimedRun{ID: uuid.Must(uuid.NewV7()), TenantID: uuid.Must(uuid.NewV7())}
	got, err := service.Evaluate(context.Background(), run, time.Now())
	if err != nil || got.Code != decision.Code || metrics.outcome != "DEFER" || metrics.code != decision.Code {
		t.Fatalf("decision=%#v metrics=%#v err=%v", got, metrics, err)
	}
	if !strings.Contains(output.String(), `"reason_code":"TENANT_CPU_LIMIT"`) {
		t.Fatalf("decision log=%s", output.String())
	}
}

func TestEligibilityServiceDoesNotLogRawRepositoryError(t *testing.T) {
	var output bytes.Buffer
	secret := errors.New("password=do-not-log")
	service := NewEligibilityService(fakeEligibility{err: secret}, nil, slog.New(slog.NewJSONHandler(&output, nil)))
	if _, err := service.Evaluate(context.Background(), ports.ClaimedRun{}, time.Now()); !errors.Is(err, secret) {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(output.String(), "do-not-log") || !strings.Contains(output.String(), "eligibility persistence failed") {
		t.Fatalf("unsafe log=%s", output.String())
	}
}
