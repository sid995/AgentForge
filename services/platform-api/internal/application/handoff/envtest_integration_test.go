//go:build controllerintegration

package handoff

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
)

func TestHandoffCreatesSchemaValidatedAgentRunInRealAPIServer(t *testing.T) {
	scheme, err := Scheme()
	if err != nil {
		t.Fatal(err)
	}
	environment := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "..", "..", "operator", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	restConfiguration, err := environment.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if stopErr := environment.Stop(); stopErr != nil {
			t.Errorf("stop envtest: %v", stopErr)
		}
	}()
	kubernetes, err := client.New(restConfiguration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	marker := &failOnceMarker{}
	service, err := NewService(testAuthorizer{}, testSelector{kubernetes}, testIntentLoader{desired: handoffDesired()}, marker, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	event := scheduledFixture(t)
	if err := service.Process(context.Background(), event, time.Now().UTC()); err == nil {
		t.Fatal("first process unexpectedly persisted its marker")
	}
	if err := service.Process(context.Background(), event, time.Now().UTC()); err != nil {
		t.Fatalf("real API crash-window replay: %v", err)
	}
	var payload events.AgentRunScheduledPayload
	if err := json.Unmarshal(event.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	run := &executionv1alpha1.AgentRun{}
	if err := kubernetes.Get(context.Background(), client.ObjectKey{Namespace: NamespaceName(event.Envelope.TenantID), Name: AgentRunName(payload.RunID, payload.AttemptNumber)}, run); err != nil {
		t.Fatal(err)
	}
	if run.Status.Phase != "" || run.Spec.RunnerImage != handoffDesired().RunnerImage {
		t.Fatalf("unexpected AgentRun: %#v", run)
	}
}
