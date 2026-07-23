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
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/resources"
)

func TestCleanupHandlesProvisioningRunningAndSucceededDeletion(t *testing.T) {
	for _, testCase := range []struct {
		phase     executionv1alpha1.AgentRunPhase
		workspace bool
	}{
		{phase: executionv1alpha1.AgentRunPhaseProvisioning},
		{phase: executionv1alpha1.AgentRunPhaseRunning, workspace: true},
		{phase: executionv1alpha1.AgentRunPhaseSucceeded, workspace: true},
	} {
		t.Run(string(testCase.phase), func(t *testing.T) {
			run := cleanupTestRun(t, "cleanup-"+strings.ToLower(string(testCase.phase)))
			run.Status.Phase = testCase.phase
			objects := []client.Object{}
			var workspace *corev1.PersistentVolumeClaim
			if testCase.workspace {
				workspace = retainedWorkspaceForAttempt(t, run, 1)
				objects = append(objects, workspace)
			}
			kubernetesClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(objects...).Build()
			reconciler := &AgentRunReconciler{Client: kubernetesClient, Now: func() time.Time { return fixedReconcileTime }}

			complete, cleanupErr := reconciler.reconcileCleanup(context.Background(), run)
			if !complete || cleanupErr != nil {
				t.Fatalf("cleanup result: complete=%t err=%v", complete, cleanupErr)
			}
			assertCleanupCondition(t, run, metav1.ConditionFalse, "CleanupComplete")
			if workspace != nil {
				stored := &corev1.PersistentVolumeClaim{}
				if err := kubernetesClient.Get(context.Background(), client.ObjectKeyFromObject(workspace), stored); err != nil {
					t.Fatalf("retained workspace was deleted: %v", err)
				}
				if stored.Annotations[retentionStateAnnotation] != retentionStateReleased {
					t.Fatalf("retained workspace handoff: %#v", stored.Annotations)
				}
			}
		})
	}
}

func TestCleanupRetainsEveryAttemptAndToleratesPartialAbsence(t *testing.T) {
	run := cleanupTestRun(t, "cleanup-attempts")
	run.Spec.RetryPolicy.MaxAttempts = 3
	run.Status.Attempt = 3
	first := retainedWorkspaceForAttempt(t, run, 1)
	third := retainedWorkspaceForAttempt(t, run, 3)
	kubernetesClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(first, third).Build()
	reconciler := &AgentRunReconciler{Client: kubernetesClient, Now: func() time.Time { return fixedReconcileTime }}

	complete, cleanupErr := reconciler.reconcileCleanup(context.Background(), run)
	if !complete || cleanupErr != nil {
		t.Fatalf("cleanup partial attempts: complete=%t err=%v", complete, cleanupErr)
	}
	for _, workspace := range []*corev1.PersistentVolumeClaim{first, third} {
		stored := &corev1.PersistentVolumeClaim{}
		if err := kubernetesClient.Get(context.Background(), client.ObjectKeyFromObject(workspace), stored); err != nil {
			t.Fatalf("get retained workspace: %v", err)
		}
		if stored.Annotations[retentionStateAnnotation] != retentionStateReleased {
			t.Fatalf("attempt workspace was not released: %#v", stored.Annotations)
		}
	}
}

func TestCleanupPartialFailureIsIdempotentAndRetryable(t *testing.T) {
	run := cleanupTestRun(t, "cleanup-partial-failure")
	run.Spec.RetryPolicy.MaxAttempts = 2
	run.Status.Attempt = 2
	first := retainedWorkspaceForAttempt(t, run, 1)
	second := retainedWorkspaceForAttempt(t, run, 2)
	baseClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(first, second).Build()
	failingClient := &failWorkspacePatchClient{Client: baseClient, failName: second.Name, remaining: 1}
	reconciler := &AgentRunReconciler{Client: failingClient, Now: func() time.Time { return fixedReconcileTime }}

	complete, cleanupErr := reconciler.reconcileCleanup(context.Background(), run)
	if complete || cleanupErr == nil || cleanupErr.Class != ErrorClassTransient {
		t.Fatalf("partial cleanup failure: complete=%t err=%v", complete, cleanupErr)
	}
	assertCleanupCondition(t, run, metav1.ConditionTrue, "RetentionHandoffFailed")
	storedFirst := &corev1.PersistentVolumeClaim{}
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(first), storedFirst); err != nil ||
		storedFirst.Annotations[retentionStateAnnotation] != retentionStateReleased {
		t.Fatalf("successful partial handoff was not retained: pvc=%#v err=%v", storedFirst, err)
	}

	complete, cleanupErr = reconciler.reconcileCleanup(context.Background(), run)
	if !complete || cleanupErr != nil {
		t.Fatalf("cleanup retry did not converge: complete=%t err=%v", complete, cleanupErr)
	}
	assertCleanupCondition(t, run, metav1.ConditionFalse, "CleanupComplete")
}

func TestCleanupConflictEscalatesAfterRestartDeadlineWithoutDeletingWorkspace(t *testing.T) {
	run := cleanupTestRun(t, "cleanup-escalation")
	workspace := retainedWorkspaceForAttempt(t, run, 1)
	workspace.Annotations[ownerUIDAnnotation] = "different-run"
	baseClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(workspace).Build()
	first := &AgentRunReconciler{Client: baseClient, Now: func() time.Time { return fixedReconcileTime }}

	complete, cleanupErr := first.reconcileCleanup(context.Background(), run)
	if complete || cleanupErr == nil || cleanupErr.Class != ErrorClassPermanent {
		t.Fatalf("cleanup conflict: complete=%t err=%v", complete, cleanupErr)
	}
	assertCleanupCondition(t, run, metav1.ConditionTrue, "RetainedWorkspaceConflict")

	restarted := &AgentRunReconciler{Client: baseClient, Now: func() time.Time { return fixedReconcileTime.Add(cleanupDeadline + time.Second) }}
	complete, cleanupErr = restarted.reconcileCleanup(context.Background(), run)
	if !complete || cleanupErr != nil {
		t.Fatalf("cleanup escalation: complete=%t err=%v", complete, cleanupErr)
	}
	assertCleanupCondition(t, run, metav1.ConditionFalse, "CleanupEscalated")
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(workspace), &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("escalation deleted retained workspace: %v", err)
	}
}

func TestFinalizerRemovalFailureRemainsDiagnosableAndRetryable(t *testing.T) {
	run := cleanupTestRun(t, "cleanup-finalizer-retry")
	run.Finalizers = []string{RetainedResourcesFinalizer}
	deletionTime := metav1.NewTime(fixedReconcileTime)
	run.DeletionTimestamp = &deletionTime
	baseClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithStatusSubresource(&executionv1alpha1.AgentRun{}).WithObjects(run).Build()
	failingClient := &failFinalizerPatchClient{Client: baseClient, remaining: 1}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(run)}
	first := &AgentRunReconciler{Client: failingClient, Now: func() time.Time { return fixedReconcileTime }}

	result, err := first.Reconcile(context.Background(), request)
	if err != nil || result.RequeueAfter != cleanupRequeue {
		t.Fatalf("failed finalizer removal: result=%#v err=%v", result, err)
	}
	stored := &executionv1alpha1.AgentRun{}
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(run), stored); err != nil {
		t.Fatalf("get cleanup-pending AgentRun: %v", err)
	}
	assertCleanupCondition(t, stored, metav1.ConditionTrue, "FinalizerRemovalFailed")
	if !containsString(stored.Finalizers, RetainedResourcesFinalizer) {
		t.Fatal("failed metadata patch unexpectedly removed finalizer")
	}

	restarted := &AgentRunReconciler{Client: failingClient, Now: func() time.Time { return fixedReconcileTime.Add(time.Second) }}
	if _, err := restarted.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("retry finalizer removal: %v", err)
	}
	err = baseClient.Get(context.Background(), client.ObjectKeyFromObject(run), &executionv1alpha1.AgentRun{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get finalized AgentRun: %v", err)
	}
	if err == nil {
		stored = &executionv1alpha1.AgentRun{}
		if getErr := baseClient.Get(context.Background(), client.ObjectKeyFromObject(run), stored); getErr != nil || containsString(stored.Finalizers, RetainedResourcesFinalizer) {
			t.Fatalf("finalizer retry did not converge: run=%#v err=%v", stored, getErr)
		}
	}
}

func cleanupTestRun(t *testing.T, name string) *executionv1alpha1.AgentRun {
	t.Helper()
	run := validControllerAgentRun(name)
	run.UID = types.UID("run-" + name)
	run.Spec.Workspace.RetentionPolicy = executionv1alpha1.WorkspaceRetentionRetain
	run.Status.Attempt = run.Spec.Attempt
	run.Status.Namespace = run.Namespace
	return run
}

func retainedWorkspaceForAttempt(t *testing.T, run *executionv1alpha1.AgentRun, attempt int32) *corev1.PersistentVolumeClaim {
	t.Helper()
	copy := run.DeepCopy()
	copy.Status.Attempt = attempt
	workspace, err := resources.NewDefaultBuilder().BuildPVC(copy)
	if err != nil {
		t.Fatalf("build retained workspace: %v", err)
	}
	return workspace
}

func assertCleanupCondition(t *testing.T, run *executionv1alpha1.AgentRun, status metav1.ConditionStatus, reason string) {
	t.Helper()
	condition := meta.FindStatusCondition(run.Status.Conditions, ConditionCleanupPending)
	if condition == nil || condition.Status != status || condition.Reason != reason {
		t.Fatalf("cleanup condition: %#v", condition)
	}
}

type failWorkspacePatchClient struct {
	client.Client
	failName  string
	remaining int
}

type failFinalizerPatchClient struct {
	client.Client
	remaining int
}

func (c *failFinalizerPatchClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	if _, ok := object.(*executionv1alpha1.AgentRun); ok && c.remaining > 0 {
		c.remaining--
		return apierrors.NewConflict(schema.GroupResource{Group: executionv1alpha1.GroupVersion.Group, Resource: "agentruns"}, object.GetName(), errors.New("simulated finalizer conflict"))
	}
	return c.Client.Patch(ctx, object, patch, options...)
}

func (c *failWorkspacePatchClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	if _, ok := object.(*corev1.PersistentVolumeClaim); ok && object.GetName() == c.failName && c.remaining > 0 {
		c.remaining--
		return apierrors.NewServiceUnavailable("retained workspace API unavailable")
	}
	return c.Client.Patch(ctx, object, patch, options...)
}
