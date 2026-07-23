// Package handoff projects committed Scheduler intent into the selected
// Kubernetes cluster without creating execution resources directly.
package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	consumerapp "github.com/sid995/agentforge/services/platform-api/internal/application/consumer"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/events"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

const ConsumerIdentity = "agentrun-handoff.v1"

type ClusterAuthorizer interface {
	Authorize(context.Context, string, uuid.UUID) error
}

type ClientSelector interface {
	Select(context.Context, string) (client.Client, error)
}

type IntentLoader interface {
	LoadIntent(context.Context, ports.ReceivedEvent, events.AgentRunScheduledPayload) (domain.AgentRunDesiredState, error)
}

type ProcessedMarker interface {
	Seen(context.Context, string, uuid.UUID, uuid.UUID) (bool, error)
	Mark(context.Context, string, ports.ReceivedEvent, time.Time) (bool, error)
}

type Metrics interface {
	Processed(string)
}

type noMetrics struct{}

func (noMetrics) Processed(string) {}

type Service struct {
	authorizer ClusterAuthorizer
	clients    ClientSelector
	intents    IntentLoader
	marker     ProcessedMarker
	metrics    Metrics
	logger     *slog.Logger
}

func NewService(authorizer ClusterAuthorizer, clients ClientSelector, intents IntentLoader, marker ProcessedMarker, metrics Metrics, logger *slog.Logger) (*Service, error) {
	if authorizer == nil || clients == nil || intents == nil || marker == nil {
		return nil, fmt.Errorf("handoff authorizer, client selector, intent loader, and marker are required")
	}
	if metrics == nil {
		metrics = noMetrics{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{authorizer: authorizer, clients: clients, intents: intents, marker: marker, metrics: metrics, logger: logger}, nil
}

// Process creates or compares the deterministic namespace and AgentRun before
// durably marking the event. A crash before the marker safely repeats compares.
func (service *Service) Process(ctx context.Context, received ports.ReceivedEvent, now time.Time) error {
	if received.Envelope.EventType != events.AgentRunScheduledType {
		return consumerapp.NewPermanentFailure("UNSUPPORTED_EVENT", "handoff accepts only scheduled AgentRun events", nil)
	}
	var payload events.AgentRunScheduledPayload
	if err := json.Unmarshal(received.Envelope.Payload, &payload); err != nil {
		return consumerapp.NewPermanentFailure("INVALID_INTENT_EVENT", "scheduled event payload is invalid", err)
	}
	seen, err := service.marker.Seen(ctx, ConsumerIdentity, received.Envelope.EventID, received.Envelope.TenantID)
	if err != nil {
		return consumerapp.NewTransientFailure("PROCESSED_MARKER", "processed-event marker is unavailable", err)
	}
	if seen {
		service.metrics.Processed("duplicate")
		service.logger.InfoContext(ctx, "scheduled AgentRun handoff already processed", "event_id", received.Envelope.EventID)
		return nil
	}
	desired, err := service.intents.LoadIntent(ctx, received, payload)
	if err != nil {
		if errors.Is(err, ErrIntentMissing) || errors.Is(err, ErrIntentInvalid) {
			return consumerapp.NewPermanentFailure("HANDOFF_INTENT", "durable AgentRun handoff intent is invalid", err)
		}
		return consumerapp.NewTransientFailure("HANDOFF_INTENT", "durable AgentRun handoff intent is unavailable", err)
	}
	if err := desired.Validate(payload.AttemptNumber); err != nil ||
		desired.ExecutionProfile != payload.ExecutionProfile ||
		desired.CPUMillis != payload.CPUMillis ||
		desired.MemoryMiB != payload.MemoryMiB {
		return consumerapp.NewPermanentFailure("HANDOFF_INTENT", "durable AgentRun handoff intent is invalid", ErrIntentInvalid)
	}
	if err := service.authorizer.Authorize(ctx, payload.ClusterID, received.Envelope.TenantID); err != nil {
		return classifyDependency("CLUSTER_AUTHORIZATION", "selected cluster is not authorized for tenant", err)
	}
	kubernetes, err := service.clients.Select(ctx, payload.ClusterID)
	if err != nil {
		return classifyDependency("CLUSTER_CLIENT", "selected cluster client is unavailable", err)
	}
	namespace := NamespaceName(received.Envelope.TenantID)
	resourceName := AgentRunName(payload.RunID, payload.AttemptNumber)
	if err := ensureNamespace(ctx, kubernetes, namespace, received.Envelope); err != nil {
		return classifyKubernetes("NAMESPACE", "tenant namespace handoff failed", err)
	}
	run := desiredAgentRun(namespace, resourceName, received.Envelope, payload, desired)
	if err := ensureAgentRun(ctx, kubernetes, run); err != nil {
		return classifyKubernetes("AGENTRUN", "AgentRun handoff failed", err)
	}
	duplicate, err := service.marker.Mark(ctx, ConsumerIdentity, received, now.UTC())
	if err != nil {
		return consumerapp.NewTransientFailure("PROCESSED_MARKER", "processed-event marker is unavailable", err)
	}
	result := "created"
	if duplicate {
		result = "duplicate"
	}
	service.metrics.Processed(result)
	service.logger.InfoContext(ctx, "scheduled AgentRun handed to Kubernetes",
		"cluster_id", payload.ClusterID, "namespace", namespace, "agent_run", resourceName,
		"event_id", received.Envelope.EventID, "tenant_id", received.Envelope.TenantID,
		"project_id", payload.ProjectID, "run_id", payload.RunID, "attempt", payload.AttemptNumber,
		"correlation_id", received.Envelope.CorrelationID, "causation_id", received.Envelope.CausationID,
		"traceparent", received.Envelope.Traceparent, "result", result)
	return nil
}

func NamespaceName(tenantID uuid.UUID) string {
	return "af-" + strings.ReplaceAll(tenantID.String(), "-", "")
}

func AgentRunName(runID uuid.UUID, attempt int) string {
	return "run-" + strings.ReplaceAll(runID.String(), "-", "") + "-a" + strconv.Itoa(attempt)
}

func ensureNamespace(ctx context.Context, kubernetes client.Client, name string, envelope events.Envelope) error {
	existing := &corev1.Namespace{}
	err := kubernetes.Get(ctx, client.ObjectKey{Name: name}, existing)
	if apierrors.IsNotFound(err) {
		createErr := kubernetes.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":       "agentforge-handoff",
				"execution.agentforge.dev/tenant-id": envelope.TenantID.String(),
			},
		}})
		if createErr == nil {
			return nil
		}
		if !apierrors.IsAlreadyExists(createErr) {
			return createErr
		}
		if err := kubernetes.Get(ctx, client.ObjectKey{Name: name}, existing); err != nil {
			return err
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if existing.Labels["app.kubernetes.io/managed-by"] != "agentforge-handoff" ||
		existing.Labels["execution.agentforge.dev/tenant-id"] != envelope.TenantID.String() {
		return fmt.Errorf("namespace tenant ownership conflicts")
	}
	return nil
}

func ensureAgentRun(ctx context.Context, kubernetes client.Client, desired *executionv1alpha1.AgentRun) error {
	existing := &executionv1alpha1.AgentRun{}
	err := kubernetes.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		createErr := kubernetes.Create(ctx, desired)
		if createErr == nil {
			return nil
		}
		if !apierrors.IsAlreadyExists(createErr) {
			return createErr
		}
		if err := kubernetes.Get(ctx, client.ObjectKeyFromObject(desired), existing); err != nil {
			return err
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(existing.Spec, desired.Spec) || !managedMetadataMatches(existing, desired) {
		return fmt.Errorf("existing AgentRun desired state conflicts")
	}
	return nil
}

func managedMetadataMatches(existing, desired *executionv1alpha1.AgentRun) bool {
	for _, key := range []string{
		"app.kubernetes.io/managed-by",
		"execution.agentforge.dev/tenant-id",
		"execution.agentforge.dev/project-id",
		"execution.agentforge.dev/run-id",
	} {
		if existing.Labels[key] != desired.Labels[key] {
			return false
		}
	}
	for key, value := range desired.Annotations {
		if existing.Annotations[key] != value {
			return false
		}
	}
	return true
}

func desiredAgentRun(namespace, name string, envelope events.Envelope, payload events.AgentRunScheduledPayload, desired domain.AgentRunDesiredState) *executionv1alpha1.AgentRun {
	run := &executionv1alpha1.AgentRun{
		TypeMeta: metav1.TypeMeta{APIVersion: executionv1alpha1.GroupVersion.String(), Kind: "AgentRun"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace, Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":        "agentforge-handoff",
				"execution.agentforge.dev/tenant-id":  envelope.TenantID.String(),
				"execution.agentforge.dev/project-id": payload.ProjectID.String(),
				"execution.agentforge.dev/run-id":     payload.RunID.String(),
			},
			Annotations: auditAnnotations(envelope, payload),
		},
		Spec: executionv1alpha1.AgentRunSpec{
			TenantID: executionv1alpha1.UUIDv7(envelope.TenantID.String()), ProjectID: executionv1alpha1.UUIDv7(payload.ProjectID.String()),
			RunID: executionv1alpha1.UUIDv7(payload.RunID.String()), AttemptID: executionv1alpha1.UUIDv7(payload.AttemptID.String()),
			Attempt: int32(payload.AttemptNumber), RunnerImage: desired.RunnerImage, Runtime: desired.Runtime,
			ExecutionProfile: desired.ExecutionProfile, TaskRef: desired.TaskReference, TimeoutSeconds: int32(desired.TimeoutSeconds),
			RetryPolicy: executionv1alpha1.RetryPolicy{
				MaxAttempts: int32(desired.MaxAttempts), InitialBackoffSeconds: int32(desired.InitialBackoffSeconds),
				MaxBackoffSeconds: int32(desired.MaxBackoffSeconds), RetryableFailureCategories: failureCategories(desired.RetryableFailureCategories),
			},
			Resources: executionv1alpha1.ResourceRequirements{
				Requests: executionv1alpha1.ResourceValues{CPUMillis: int32(payload.CPUMillis), MemoryMiB: int32(payload.MemoryMiB)},
				Limits:   executionv1alpha1.ResourceValues{CPUMillis: int32(desired.CPULimitMillis), MemoryMiB: int32(desired.MemoryLimitMiB)},
			},
			Workspace:              executionv1alpha1.WorkspaceSpec{SizeGiB: int32(desired.WorkspaceSizeGiB), StorageClassName: desired.StorageClassName, RetentionPolicy: executionv1alpha1.WorkspaceRetentionPolicy(desired.WorkspaceRetentionPolicy)},
			Network:                executionv1alpha1.NetworkSpec{Profile: executionv1alpha1.NetworkProfile(desired.NetworkProfile), AllowedDestinationsRef: optionalReference(desired.AllowedDestinationsRef)},
			ArtifactDestinationRef: executionv1alpha1.LocalObjectReference{Name: desired.ArtifactDestinationRef},
			ConfigurationRefs:      references(desired.ConfigurationRefs), SecretRefs: references(desired.SecretRefs),
			DeployOnSuccess: desired.DeployOnSuccess, DesiredState: executionv1alpha1.DesiredStateRunning,
		},
	}
	return run
}

func auditAnnotations(envelope events.Envelope, payload events.AgentRunScheduledPayload) map[string]string {
	annotations := map[string]string{
		"execution.agentforge.dev/scheduled-event-id": envelope.EventID.String(),
		"execution.agentforge.dev/correlation-id":     envelope.CorrelationID,
		"execution.agentforge.dev/causation-id":       envelope.CausationID,
		"execution.agentforge.dev/cluster-id":         payload.ClusterID,
	}
	if envelope.Traceparent != "" {
		annotations["execution.agentforge.dev/traceparent"] = envelope.Traceparent
	}
	if envelope.Tracestate != "" {
		annotations["execution.agentforge.dev/tracestate"] = envelope.Tracestate
	}
	return annotations
}

func references(values []string) []executionv1alpha1.LocalObjectReference {
	result := make([]executionv1alpha1.LocalObjectReference, len(values))
	for index, value := range values {
		result[index] = executionv1alpha1.LocalObjectReference{Name: value}
	}
	return result
}

func optionalReference(value string) *executionv1alpha1.LocalObjectReference {
	if value == "" {
		return nil
	}
	return &executionv1alpha1.LocalObjectReference{Name: value}
}

func failureCategories(values []domain.FailureCategory) []executionv1alpha1.FailureCategory {
	result := make([]executionv1alpha1.FailureCategory, len(values))
	for index, value := range values {
		result[index] = executionv1alpha1.FailureCategory(value)
	}
	return result
}

func classifyDependency(code, reason string, err error) error {
	if errors.Is(err, ErrUnauthorizedCluster) {
		return consumerapp.NewPermanentFailure(code, reason, err)
	}
	return consumerapp.NewTransientFailure(code, reason, err)
}

func classifyKubernetes(code, reason string, err error) error {
	if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) ||
		apierrors.IsAlreadyExists(err) || strings.Contains(err.Error(), "conflicts") {
		return consumerapp.NewPermanentFailure(code+"_CONFLICT", reason, err)
	}
	return consumerapp.NewTransientFailure(code+"_UNAVAILABLE", reason, err)
}

var ErrUnauthorizedCluster = errors.New("cluster is not registered or tenant-authorized")
var ErrIntentMissing = errors.New("durable handoff intent is missing")
var ErrIntentInvalid = errors.New("durable handoff intent is invalid")

// Scheme returns the exact API scheme needed by handoff clients and tests.
func Scheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := executionv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}
