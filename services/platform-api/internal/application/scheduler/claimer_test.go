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

	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type fakeQueue struct {
	claimed []ports.ClaimedRun
	err     error
}

func (queue *fakeQueue) Claim(context.Context, ports.SchedulerClaimRequest) ([]ports.ClaimedRun, error) {
	return queue.claimed, queue.err
}

func (*fakeQueue) Renew(context.Context, uuid.UUID, uuid.UUID, string, int64, time.Time, time.Duration) (int64, error) {
	return 2, nil
}

type claimMetrics struct {
	result string
	count  int
	age    time.Duration
}

func (metrics *claimMetrics) Claimed(result string, count int) {
	metrics.result, metrics.count = result, count
}
func (metrics *claimMetrics) QueueAge(age time.Duration) { metrics.age = age }
func (*claimMetrics) LeaseRenewed(string)                {}

func TestClaimerRecordsQueueAgeAndStructuredOutcome(t *testing.T) {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	metrics := &claimMetrics{}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	claimer := NewClaimer(&fakeQueue{claimed: []ports.ClaimedRun{{CreatedAt: now.Add(-3 * time.Minute)}, {CreatedAt: now.Add(-time.Minute)}}}, metrics, logger)
	claimed, err := claimer.Claim(context.Background(), ports.SchedulerClaimRequest{Owner: "scheduler-a", Now: now})
	if err != nil || len(claimed) != 2 || metrics.result != "claimed" || metrics.count != 2 || metrics.age != 3*time.Minute {
		t.Fatalf("claimed=%d metrics=%#v err=%v", len(claimed), metrics, err)
	}
	logLine := output.String()
	if !strings.Contains(logLine, `"result":"claimed"`) || !strings.Contains(logLine, `"oldest_queue_age_ms":180000`) {
		t.Fatalf("structured claim log=%s", logLine)
	}
}

func TestClaimerReturnsEmptyWithoutErrorAndSanitizesDatabaseFailureLog(t *testing.T) {
	metrics := &claimMetrics{}
	var output bytes.Buffer
	claimer := NewClaimer(&fakeQueue{}, metrics, slog.New(slog.NewJSONHandler(&output, nil)))
	claimed, err := claimer.Claim(context.Background(), ports.SchedulerClaimRequest{Owner: "scheduler-a", Now: time.Now()})
	if err != nil || len(claimed) != 0 || metrics.result != "empty" {
		t.Fatalf("claimed=%#v metrics=%#v err=%v", claimed, metrics, err)
	}

	secret := errors.New("postgres password=do-not-log")
	output.Reset()
	claimer = NewClaimer(&fakeQueue{err: secret}, metrics, slog.New(slog.NewJSONHandler(&output, nil)))
	if _, err := claimer.Claim(context.Background(), ports.SchedulerClaimRequest{Owner: "scheduler-a", Now: time.Now()}); !errors.Is(err, secret) {
		t.Fatalf("claim error=%v", err)
	}
	if strings.Contains(output.String(), "do-not-log") || !strings.Contains(output.String(), "database claim failed") {
		t.Fatalf("unsafe failure log=%s", output.String())
	}
}
