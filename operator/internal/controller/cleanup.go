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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

const (
	cleanupRequeue  = 15 * time.Second
	cleanupDeadline = 10 * time.Minute

	ownerUIDAnnotation       = "execution.agentforge.dev/owner-uid"
	retentionAnnotation      = "execution.agentforge.dev/retention-policy"
	retentionStateAnnotation = "execution.agentforge.dev/retention-state"
	retentionStateReleased   = "Released"
)

func (r *AgentRunReconciler) reconcileCleanup(ctx context.Context, run *executionv1alpha1.AgentRun) (bool, *ReconcileError) {
	r.initializeDeletingStatus(run)
	if run.Spec.Workspace.RetentionPolicy != executionv1alpha1.WorkspaceRetentionRetain {
		r.completeCleanup(run, "CleanupNotRequired", "No explicitly retained resource requires cleanup")
		return true, nil
	}

	for attempt := run.Spec.Attempt; attempt <= naming.CurrentAttempt(run); attempt++ {
		workspaceName, err := workspaceNameForAttempt(run, attempt)
		if err != nil {
			return r.handleCleanupFailure(run, "CleanupIdentityInvalid", newReconcileError(ErrorClassPermanent, "CleanupIdentityInvalid", err))
		}
		workspace := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: workspaceName}, workspace); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return r.handleCleanupFailure(run, "RetainedWorkspaceReadFailed", classifyAPIError("RetainedWorkspaceReadFailed", err))
		}
		if err := validateRetainedWorkspace(run, workspace); err != nil {
			return r.handleCleanupFailure(run, "RetainedWorkspaceConflict", newReconcileError(ErrorClassPermanent, "RetainedWorkspaceConflict", err))
		}
		if workspace.Annotations[retentionStateAnnotation] == retentionStateReleased {
			continue
		}
		before := workspace.DeepCopy()
		if workspace.Annotations == nil {
			workspace.Annotations = map[string]string{}
		}
		workspace.Annotations[retentionStateAnnotation] = retentionStateReleased
		patch := client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})
		if err := r.Patch(ctx, workspace, patch); err != nil {
			return r.handleCleanupFailure(run, "RetentionHandoffFailed", classifyAPIError("RetentionHandoffFailed", err))
		}
	}

	r.completeCleanup(run, "CleanupComplete", "Retained workspace handoff is complete")
	return true, nil
}

func (r *AgentRunReconciler) handleCleanupFailure(run *executionv1alpha1.AgentRun, reason string, cleanupErr *ReconcileError) (bool, *ReconcileError) {
	started := cleanupStartedAt(run)
	if !started.IsZero() && !r.now().Before(started.Add(cleanupDeadline)) {
		r.completeCleanup(run, "CleanupEscalated", "Cleanup deadline elapsed; finalizer release requires retained-resource operator review")
		return true, nil
	}
	message := "Retained workspace handoff could not make progress"
	r.setCondition(run, ConditionCleanupPending, metav1.ConditionTrue, reason, message)
	return false, cleanupErr
}

func (r *AgentRunReconciler) completeCleanup(run *executionv1alpha1.AgentRun, reason, message string) {
	r.setCondition(run, ConditionCleanupPending, metav1.ConditionFalse, reason, message)
}

func cleanupStartedAt(run *executionv1alpha1.AgentRun) time.Time {
	if !run.DeletionTimestamp.IsZero() {
		return run.DeletionTimestamp.Time
	}
	for index := range run.Status.Conditions {
		condition := &run.Status.Conditions[index]
		if condition.Type == ConditionCleanupPending && condition.Status == metav1.ConditionTrue {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

func workspaceNameForAttempt(run *executionv1alpha1.AgentRun, attempt int32) (string, error) {
	copy := run.DeepCopy()
	copy.Status.Attempt = attempt
	names, err := naming.ForAgentRun(copy)
	if err != nil {
		return "", err
	}
	return names.Workspace, nil
}

func validateRetainedWorkspace(run *executionv1alpha1.AgentRun, workspace *corev1.PersistentVolumeClaim) error {
	if controller := metav1.GetControllerOf(workspace); controller != nil {
		return errors.New("retained workspace unexpectedly has a controller owner")
	}
	if workspace.Annotations[ownerUIDAnnotation] != string(run.UID) {
		return fmt.Errorf("retained workspace owner identity does not match")
	}
	if workspace.Annotations[retentionAnnotation] != string(executionv1alpha1.WorkspaceRetentionRetain) {
		return fmt.Errorf("retained workspace policy marker does not match")
	}
	return nil
}
