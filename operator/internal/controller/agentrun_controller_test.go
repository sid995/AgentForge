/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/event"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

const controllerTestNamespace = "agentrun-controller"

var fixedReconcileTime = time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)

func TestAgentRunReconcilerFoundationEnvtest(t *testing.T) {
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	configuration, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})

	scheme := controllerTestScheme(t)
	baseClient, err := client.New(configuration, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create envtest client: %v", err)
	}
	recording := &recordingClient{Client: baseClient}
	reconciler := &AgentRunReconciler{Client: recording, Now: func() time.Time { return fixedReconcileTime }}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := baseClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: controllerTestNamespace}}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	t.Run("not found is a successful no-op", func(t *testing.T) {
		result, err := reconciler.Reconcile(ctx, requestFor("missing"))
		if err != nil || !result.IsZero() {
			t.Fatalf("not-found reconcile: result=%#v error=%v", result, err)
		}
	})

	t.Run("create initializes status and duplicate reconcile does not write", func(t *testing.T) {
		run := validControllerAgentRun("initialize")
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("first reconcile: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		assertInitializedStatus(t, stored)
		if len(stored.Finalizers) != 0 {
			t.Fatalf("ordinary AgentRun received an unnecessary finalizer: %v", stored.Finalizers)
		}
		writesAfterFirst := recording.statusPatches
		if writesAfterFirst == 0 {
			t.Fatal("first reconcile did not write initialized status")
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("duplicate reconcile: %v", err)
		}
		if recording.statusPatches != writesAfterFirst {
			t.Fatalf("duplicate reconcile wrote status: before=%d after=%d", writesAfterFirst, recording.statusPatches)
		}
	})

	t.Run("generation update advances observed generation", func(t *testing.T) {
		stored := getControllerAgentRun(t, ctx, baseClient, "initialize")
		oldGeneration := stored.Generation
		stored.Spec.DesiredState = executionv1alpha1.DesiredStateCancelled
		if err := baseClient.Update(ctx, stored); err != nil {
			t.Fatalf("update desired state: %v", err)
		}
		if stored.Generation <= oldGeneration {
			t.Fatalf("spec update did not advance generation: old=%d new=%d", oldGeneration, stored.Generation)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(stored.Name)); err != nil {
			t.Fatalf("reconcile new generation: %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, stored.Name)
		if stored.Status.ObservedGeneration != stored.Generation {
			t.Fatalf("observed generation=%d, metadata generation=%d", stored.Status.ObservedGeneration, stored.Generation)
		}
		for _, conditionType := range []string{ConditionSpecValid, ConditionReady} {
			condition := meta.FindStatusCondition(stored.Status.Conditions, conditionType)
			if condition == nil || condition.ObservedGeneration != stored.Generation {
				t.Fatalf("condition %q does not represent generation %d: %#v", conditionType, stored.Generation, condition)
			}
		}
	})

	t.Run("retained workspace receives finalizer", func(t *testing.T) {
		run := validControllerAgentRun("retained")
		run.Spec.Workspace.RetentionPolicy = executionv1alpha1.WorkspaceRetentionRetain
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create retained AgentRun: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile retained AgentRun: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		if !containsString(stored.Finalizers, RetainedResourcesFinalizer) {
			t.Fatalf("retained AgentRun finalizers: %v", stored.Finalizers)
		}
		metadataPatches := recording.metadataPatches
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("duplicate retained reconcile: %v", err)
		}
		if recording.metadataPatches != metadataPatches {
			t.Fatal("duplicate reconcile rewrote the finalizer")
		}
	})

	t.Run("deletion timestamp exposes cleanup pending and keeps finalizer", func(t *testing.T) {
		stored := getControllerAgentRun(t, ctx, baseClient, "retained")
		if err := baseClient.Delete(ctx, stored); err != nil {
			t.Fatalf("delete retained AgentRun: %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, stored.Name)
		if stored.DeletionTimestamp.IsZero() {
			t.Fatal("deletion timestamp was not set")
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(stored.Name)); err != nil {
			t.Fatalf("reconcile deletion: %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, stored.Name)
		condition := meta.FindStatusCondition(stored.Status.Conditions, ConditionCleanupPending)
		if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "RetainedResources" {
			t.Fatalf("unexpected cleanup condition: %#v", condition)
		}
		if !containsString(stored.Finalizers, RetainedResourcesFinalizer) {
			t.Fatal("foundation removed finalizer before retained-resource cleanup exists")
		}
	})

	t.Run("status conflict is transient and retry converges", func(t *testing.T) {
		run := validControllerAgentRun("status-conflict")
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		conflicting := &conflictOnceClient{Client: baseClient, remainingStatusConflicts: 1}
		conflictReconciler := &AgentRunReconciler{Client: conflicting, Now: func() time.Time { return fixedReconcileTime }}
		if _, err := conflictReconciler.Reconcile(ctx, requestFor(run.Name)); !apierrors.IsConflict(err) {
			t.Fatalf("expected transient status conflict, got %v", err)
		}
		if _, err := conflictReconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("retry after status conflict: %v", err)
		}
		assertInitializedStatus(t, getControllerAgentRun(t, ctx, baseClient, run.Name))
	})

	t.Run("foundation creates no execution resources", func(t *testing.T) {
		assertNoExecutionResources(t, ctx, baseClient)
	})
}

func TestInvalidSpecIsPermanentAndRecorded(t *testing.T) {
	scheme := controllerTestScheme(t)
	run := validControllerAgentRun("invalid")
	run.Generation = 1
	run.Spec.RunID = "invalid"
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&executionv1alpha1.AgentRun{}).WithObjects(run).Build()
	reconciler := &AgentRunReconciler{Client: fakeClient, Now: func() time.Time { return fixedReconcileTime }}

	_, err := reconciler.Reconcile(context.Background(), requestFor(run.Name))
	var reconcileErr *ReconcileError
	if !errors.As(err, &reconcileErr) || reconcileErr.Class != ErrorClassPermanent || reconcileErr.Reason != "InvalidSpec" {
		t.Fatalf("expected permanent InvalidSpec error, got %v", err)
	}
	stored := getControllerAgentRun(t, context.Background(), fakeClient, run.Name)
	if stored.Status.Phase != executionv1alpha1.AgentRunPhaseFailed || stored.Status.FailureCategory != executionv1alpha1.FailureCategoryValidation {
		t.Fatalf("invalid status was not recorded: %#v", stored.Status)
	}
	condition := meta.FindStatusCondition(stored.Status.Conditions, ConditionSpecValid)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "InvalidSpec" {
		t.Fatalf("unexpected spec condition: %#v", condition)
	}
}

func TestDeterministicResourceNames(t *testing.T) {
	run := validControllerAgentRun("names")
	first, err := NamesForAgentRun(run)
	if err != nil {
		t.Fatalf("derive names: %v", err)
	}
	second, err := NamesForAgentRun(run.DeepCopy())
	if err != nil || first != second {
		t.Fatalf("names are not deterministic: first=%#v second=%#v error=%v", first, second, err)
	}
	for kind, name := range map[string]string{
		"service account": first.ServiceAccount, "configuration": first.Configuration,
		"workspace": first.Workspace, "network policy": first.NetworkPolicy, "job": first.Job,
	} {
		if len(name) > 63 || name == "" {
			t.Errorf("%s name is not a DNS label: %q", kind, name)
		}
	}
	run.Spec.RetryPolicy.MaxAttempts = 2
	run.Status.Attempt = 2
	nextAttempt, err := NamesForAgentRun(run)
	if err != nil {
		t.Fatalf("derive retry names: %v", err)
	}
	if first.Job == nextAttempt.Job {
		t.Fatal("different attempts produced the same Job name")
	}
}

func TestBoundedExponentialRateLimiter(t *testing.T) {
	limiter := newRateLimiter()
	request := requestFor("backoff")
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 64 * time.Second, 2 * time.Minute, 2 * time.Minute}
	for index, expected := range want {
		if got := limiter.When(request); got != expected {
			t.Fatalf("backoff %d: want %s, got %s", index, expected, got)
		}
	}
	limiter.Forget(request)
	if got := limiter.When(request); got != minimumRetryDelay {
		t.Fatalf("forgotten request did not reset: %s", got)
	}
}

func TestAPIErrorClassification(t *testing.T) {
	conflict := apierrors.NewConflict(schema.GroupResource{Group: "execution.agentforge.dev", Resource: "agentruns"}, "run", errors.New("conflict"))
	if got := classifyAPIError("WriteFailed", conflict).Class; got != ErrorClassTransient {
		t.Fatalf("conflict class: %q", got)
	}
	forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "execution.agentforge.dev", Resource: "agentruns"}, "run", errors.New("forbidden"))
	if got := classifyAPIError("WriteFailed", forbidden).Class; got != ErrorClassPermanent {
		t.Fatalf("forbidden class: %q", got)
	}
	if got := classifyAPIError("ReadFailed", errors.New("connection reset")).Class; got != ErrorClassTransient {
		t.Fatalf("unknown transport failure class: %q", got)
	}
}

func TestReconciliationPredicateSuppressesSelfUpdates(t *testing.T) {
	predicate := reconciliationPredicate()
	oldRun := validControllerAgentRun("predicate")
	oldRun.Generation = 1
	newRun := oldRun.DeepCopy()
	newRun.Status.Phase = executionv1alpha1.AgentRunPhasePending
	if predicate.Update(event.UpdateEvent{ObjectOld: oldRun, ObjectNew: newRun}) {
		t.Fatal("status-only update should not enqueue reconciliation")
	}
	newRun = oldRun.DeepCopy()
	newRun.Generation = 2
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldRun, ObjectNew: newRun}) {
		t.Fatal("spec generation update should enqueue reconciliation")
	}
	newRun = oldRun.DeepCopy()
	deletionTime := metav1.NewTime(fixedReconcileTime)
	newRun.DeletionTimestamp = &deletionTime
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldRun, ObjectNew: newRun}) {
		t.Fatal("deletion timestamp update should enqueue reconciliation")
	}
}

func controllerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for name, add := range map[string]func(*runtime.Scheme) error{
		"core": corev1.AddToScheme, "batch": batchv1.AddToScheme, "networking": networkingv1.AddToScheme,
		"AgentRun": executionv1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("register %s scheme: %v", name, err)
		}
	}
	return scheme
}

func validControllerAgentRun(name string) *executionv1alpha1.AgentRun {
	return &executionv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: controllerTestNamespace},
		Spec: executionv1alpha1.AgentRunSpec{
			TenantID:         "019b0000-0000-7000-8000-000000000002",
			ProjectID:        "019b0000-0000-7000-8000-000000000003",
			RunID:            "019b0000-0000-7000-8000-000000000004",
			AttemptID:        "019b0000-0000-7000-8000-000000000005",
			Attempt:          1,
			RunnerImage:      "ghcr.io/agentforge/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Runtime:          "python-3.12",
			ExecutionProfile: "standard",
			TaskRef:          "vault://tasks/run-example",
			TimeoutSeconds:   1800,
			RetryPolicy: executionv1alpha1.RetryPolicy{
				MaxAttempts:           1,
				InitialBackoffSeconds: 5,
				MaxBackoffSeconds:     300,
			},
			Resources: executionv1alpha1.ResourceRequirements{
				Requests: executionv1alpha1.ResourceValues{CPUMillis: 1000, MemoryMiB: 2048},
				Limits:   executionv1alpha1.ResourceValues{CPUMillis: 2000, MemoryMiB: 4096},
			},
			Workspace: executionv1alpha1.WorkspaceSpec{SizeGiB: 20, RetentionPolicy: executionv1alpha1.WorkspaceRetentionDelete},
			Network:   executionv1alpha1.NetworkSpec{Profile: executionv1alpha1.NetworkProfileIsolated},
			ArtifactDestinationRef: executionv1alpha1.LocalObjectReference{
				Name: "artifact-store",
			},
			DesiredState: executionv1alpha1.DesiredStateRunning,
		},
	}
}

func requestFor(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: controllerTestNamespace, Name: name}}
}

func getControllerAgentRun(t *testing.T, ctx context.Context, kubernetesClient client.Client, name string) *executionv1alpha1.AgentRun {
	t.Helper()
	run := &executionv1alpha1.AgentRun{}
	if err := kubernetesClient.Get(ctx, requestFor(name).NamespacedName, run); err != nil {
		t.Fatalf("get AgentRun %q: %v", name, err)
	}
	return run
}

func assertInitializedStatus(t *testing.T, run *executionv1alpha1.AgentRun) {
	t.Helper()
	if run.Status.Phase != executionv1alpha1.AgentRunPhasePending || run.Status.Attempt != run.Spec.Attempt ||
		run.Status.Namespace != run.Namespace || run.Status.ObservedGeneration != run.Generation {
		t.Fatalf("unexpected initialized status: %#v", run.Status)
	}
	specValid := meta.FindStatusCondition(run.Status.Conditions, ConditionSpecValid)
	ready := meta.FindStatusCondition(run.Status.Conditions, ConditionReady)
	if specValid == nil || specValid.Status != metav1.ConditionTrue || specValid.Reason != "Accepted" {
		t.Fatalf("unexpected SpecValid condition: %#v", specValid)
	}
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "ReconciliationPending" {
		t.Fatalf("unexpected Ready condition: %#v", ready)
	}
	if !specValid.LastTransitionTime.Equal(&metav1.Time{Time: fixedReconcileTime}) {
		t.Fatalf("condition did not use injected clock: %s", specValid.LastTransitionTime)
	}
}

func assertNoExecutionResources(t *testing.T, ctx context.Context, kubernetesClient client.Client) {
	t.Helper()
	lists := []client.ObjectList{
		&batchv1.JobList{}, &corev1.ServiceAccountList{}, &corev1.ConfigMapList{},
		&corev1.PersistentVolumeClaimList{}, &networkingv1.NetworkPolicyList{},
	}
	for _, list := range lists {
		if err := kubernetesClient.List(ctx, list, client.InNamespace(controllerTestNamespace)); err != nil {
			t.Fatalf("list %T: %v", list, err)
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			t.Fatalf("extract %T: %v", list, err)
		}
		if len(items) != 0 {
			t.Fatalf("foundation unexpectedly created %d %T objects", len(items), list)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type recordingClient struct {
	client.Client
	metadataPatches int
	statusPatches   int
}

func (c *recordingClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	c.metadataPatches++
	return c.Client.Patch(ctx, object, patch, options...)
}

func (c *recordingClient) Status() client.SubResourceWriter {
	return &recordingStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type recordingStatusWriter struct {
	client.SubResourceWriter
	parent *recordingClient
}

func (w *recordingStatusWriter) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.SubResourcePatchOption) error {
	w.parent.statusPatches++
	return w.SubResourceWriter.Patch(ctx, object, patch, options...)
}

type conflictOnceClient struct {
	client.Client
	remainingStatusConflicts int
}

func (c *conflictOnceClient) Status() client.SubResourceWriter {
	return &conflictOnceStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type conflictOnceStatusWriter struct {
	client.SubResourceWriter
	parent *conflictOnceClient
}

func (w *conflictOnceStatusWriter) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.SubResourcePatchOption) error {
	if w.parent.remainingStatusConflicts > 0 {
		w.parent.remainingStatusConflicts--
		return apierrors.NewConflict(
			schema.GroupResource{Group: executionv1alpha1.GroupVersion.Group, Resource: "agentruns"},
			object.GetName(),
			fmt.Errorf("injected status conflict"),
		)
	}
	return w.SubResourceWriter.Patch(ctx, object, patch, options...)
}
