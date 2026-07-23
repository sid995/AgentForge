package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type testAuthorizer struct{ err error }

func (authorizer testAuthorizer) Authorize(context.Context, string, uuid.UUID) error {
	return authorizer.err
}

type testSelector struct{ client client.Client }

func (selector testSelector) Select(context.Context, string) (client.Client, error) {
	return selector.client, nil
}

type testIntentLoader struct {
	desired domain.AgentRunDesiredState
	err     error
}

func (loader testIntentLoader) LoadIntent(context.Context, ports.ReceivedEvent, events.AgentRunScheduledPayload) (domain.AgentRunDesiredState, error) {
	return loader.desired, loader.err
}

type testMarker struct{ calls int }

func (marker *testMarker) Seen(context.Context, string, uuid.UUID, uuid.UUID) (bool, error) {
	return marker.calls > 0, nil
}

func (marker *testMarker) Mark(context.Context, string, ports.ReceivedEvent, time.Time) (bool, error) {
	marker.calls++
	return false, nil
}

type failOnceMarker struct{ calls int }

func (*failOnceMarker) Seen(context.Context, string, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

func (marker *failOnceMarker) Mark(context.Context, string, ports.ReceivedEvent, time.Time) (bool, error) {
	marker.calls++
	if marker.calls == 1 {
		return false, errors.New("marker unavailable")
	}
	return false, nil
}

func TestServiceCreatesAndIdempotentlyComparesAgentRunWithoutStatus(t *testing.T) {
	ctx := context.Background()
	scheme, err := Scheme()
	if err != nil {
		t.Fatal(err)
	}
	kubernetes := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&executionv1alpha1.AgentRun{}).Build()
	marker := &testMarker{}
	service, err := NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, marker, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	event := scheduledFixture(t)
	if err := service.Process(ctx, event, time.Now().UTC()); err != nil {
		t.Fatalf("first process: %v", err)
	}
	if err := service.Process(ctx, event, time.Now().UTC()); err != nil {
		t.Fatalf("duplicate process: %v", err)
	}
	var payload events.AgentRunScheduledPayload
	if err := json.Unmarshal(event.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	run := &executionv1alpha1.AgentRun{}
	key := client.ObjectKey{Namespace: NamespaceName(event.Envelope.TenantID), Name: AgentRunName(payload.RunID, payload.AttemptNumber)}
	if err := kubernetes.Get(ctx, key, run); err != nil {
		t.Fatal(err)
	}
	if run.Status.Phase != "" || run.Spec.RunID != executionv1alpha1.UUIDv7(payload.RunID.String()) || marker.calls != 1 {
		t.Fatalf("run=%#v marker calls=%d", run, marker.calls)
	}
	namespace := &corev1.Namespace{}
	if err := kubernetes.Get(ctx, client.ObjectKey{Name: key.Namespace}, namespace); err != nil {
		t.Fatal(err)
	}
	if namespace.Labels["execution.agentforge.dev/tenant-id"] != event.Envelope.TenantID.String() {
		t.Fatalf("namespace labels=%v", namespace.Labels)
	}
}

func TestServiceRejectsTenantNamespaceAndExistingSpecConflicts(t *testing.T) {
	event := scheduledFixture(t)
	namespace := NamespaceName(event.Envelope.TenantID)
	scheme, _ := Scheme()
	kubernetes := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: map[string]string{"execution.agentforge.dev/tenant-id": uuid.Must(uuid.NewV7()).String()}}}).Build()
	service, _ := NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, &testMarker{}, nil, nil)
	var permanent interface{ Permanent() bool }
	err := service.Process(context.Background(), event, time.Now().UTC())
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("tenant namespace conflict was not permanent: %v", err)
	}

	kubernetes = fake.NewClientBuilder().WithScheme(scheme).Build()
	service, _ = NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, &testMarker{}, nil, nil)
	if err := service.Process(context.Background(), event, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var payload events.AgentRunScheduledPayload
	_ = json.Unmarshal(event.Envelope.Payload, &payload)
	run := &executionv1alpha1.AgentRun{}
	key := client.ObjectKey{Namespace: namespace, Name: AgentRunName(payload.RunID, payload.AttemptNumber)}
	if err := kubernetes.Get(context.Background(), key, run); err != nil {
		t.Fatal(err)
	}
	run.Spec.TimeoutSeconds++
	if err := kubernetes.Update(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	service, _ = NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, &testMarker{}, nil, nil)
	err = service.Process(context.Background(), event, time.Now().UTC())
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("spec conflict was not permanent: %v", err)
	}
}

func TestServiceRecoversFromCrashWindowByComparingExistingResources(t *testing.T) {
	ctx := context.Background()
	scheme, _ := Scheme()
	kubernetes := fake.NewClientBuilder().WithScheme(scheme).Build()
	marker := &failOnceMarker{}
	service, _ := NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, marker, nil, nil)
	event := scheduledFixture(t)
	var transient interface{ Permanent() bool }
	if err := service.Process(ctx, event, time.Now().UTC()); !errors.As(err, &transient) || transient.Permanent() {
		t.Fatalf("first marker failure was not transient: %v", err)
	}
	if err := service.Process(ctx, event, time.Now().UTC()); err != nil {
		t.Fatalf("crash-window replay failed: %v", err)
	}
	if marker.calls != 2 {
		t.Fatalf("marker calls=%d, want 2", marker.calls)
	}
}

func TestServiceRejectsUnauthorizedClusterBeforeClientSelection(t *testing.T) {
	service, _ := NewService(testAuthorizer{err: ErrUnauthorizedCluster}, testSelector{}, testIntentLoader{desired: handoffDesired()}, &testMarker{}, nil, nil)
	var permanent interface{ Permanent() bool }
	err := service.Process(context.Background(), scheduledFixture(t), time.Now().UTC())
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("unauthorized cluster was not permanent: %v", err)
	}
}

func TestServiceRejectsMissingDurableIntent(t *testing.T) {
	event := scheduledFixture(t)
	service, _ := NewService(testAuthorizer{}, testSelector{}, testIntentLoader{err: ErrIntentMissing}, &testMarker{}, nil, nil)
	var permanent interface{ Permanent() bool }
	err := service.Process(context.Background(), event, time.Now().UTC())
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("missing durable intent was not permanent: %v", err)
	}
}

func TestServiceRejectsInvalidLoadedIntentBeforeKubernetesSelection(t *testing.T) {
	event := scheduledFixture(t)
	desired := handoffDesired()
	desired.CPUMillis++
	service, _ := NewService(testAuthorizer{}, testSelector{}, testIntentLoader{desired: desired}, &testMarker{}, nil, nil)
	var permanent interface{ Permanent() bool }
	err := service.Process(context.Background(), event, time.Now().UTC())
	if !errors.As(err, &permanent) || !permanent.Permanent() {
		t.Fatalf("invalid loaded intent was not permanent: %v", err)
	}
}

func TestEnsureAgentRunRejectsMismatchedManagedMetadata(t *testing.T) {
	event := scheduledFixture(t)
	var payload events.AgentRunScheduledPayload
	if err := json.Unmarshal(event.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	desired := desiredAgentRun(NamespaceName(event.Envelope.TenantID), AgentRunName(payload.RunID, payload.AttemptNumber), event.Envelope, payload, handoffDesired())
	existing := desired.DeepCopy()
	existing.Annotations["execution.agentforge.dev/scheduled-event-id"] = uuid.Must(uuid.NewV7()).String()
	scheme, _ := Scheme()
	kubernetes := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	if err := ensureAgentRun(context.Background(), kubernetes, desired); err == nil {
		t.Fatal("managed metadata conflict was accepted")
	}
}

func handoffDesired() domain.AgentRunDesiredState {
	return domain.AgentRunDesiredState{
		RunnerImage: "registry.example.test/agentforge/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Runtime:     "python-3.12", ExecutionProfile: "standard", TaskReference: "vault://prompts/run-example",
		TimeoutSeconds: 1800, MaxAttempts: 3, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300,
		RetryableFailureCategories: []domain.FailureCategory{domain.FailureTransientDependency, domain.FailureInternal},
		CPUMillis:                  1000, MemoryMiB: 2048, CPULimitMillis: 1000, MemoryLimitMiB: 2048,
		WorkspaceSizeGiB: 10, StorageClassName: "standard", WorkspaceRetentionPolicy: "Delete",
		NetworkProfile: "Isolated", ArtifactDestinationRef: "artifact-store",
		ConfigurationRefs: []string{"runner-policy"}, SecretRefs: []string{"artifact-credential"},
	}
}

func scheduledFixture(t *testing.T) ports.ReceivedEvent {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "events", "examples", "agent-run.scheduled.v1.json")
	serialized, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := events.DecodeEnvelope(serialized)
	if err != nil {
		t.Fatal(err)
	}
	return ports.ReceivedEvent{Envelope: envelope, Topic: events.AgentRunLifecycleTopic, Partition: 0, Offset: 1, Key: []byte(envelope.AggregateID.String()), Headers: events.HeadersForEnvelope(envelope), Value: serialized}
}
