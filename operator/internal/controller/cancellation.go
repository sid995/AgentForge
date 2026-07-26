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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

const (
	cancellationDeadline = 2 * time.Minute
	cancellationRequeue  = 5 * time.Second
)

func (r *AgentRunReconciler) reconcileCancellation(ctx context.Context, run *executionv1alpha1.AgentRun) (ctrl.Result, error) {
	if run.Status.JobName == "" {
		r.completeCancellation(run, false)
		return ctrl.Result{}, nil
	}

	names, err := naming.ForAgentRun(run)
	if err != nil {
		return ctrl.Result{}, r.recordCancellationError(run, "CancellationIdentityInvalid", newReconcileError(ErrorClassPermanent, "CancellationIdentityInvalid", err))
	}
	if run.Status.JobName != names.Job {
		return ctrl.Result{}, r.recordCancellationError(run, "CancellationJobConflict", newReconcileError(ErrorClassPermanent, "CancellationJobConflict", errors.New("status Job identity does not match the current attempt")))
	}

	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: run.Status.JobName}, job); err != nil {
		if apierrors.IsNotFound(err) {
			r.completeCancellation(run, cancellationWasForced(run, r.now()))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, r.recordCancellationError(run, "CancellationJobReadFailed", err)
	}

	if job.DeletionTimestamp.IsZero() {
		pods := &corev1.PodList{}
		if err := r.List(ctx, pods, client.InNamespace(job.Namespace), client.MatchingLabels{batchv1.JobNameLabel: job.Name}); err != nil {
			return ctrl.Result{}, r.recordCancellationError(run, "CancellationPodReadFailed", err)
		}
		observation := observeLifecycle(job, pods.Items)
		if observation.phase == executionv1alpha1.AgentRunPhaseSucceeded || observation.phase == executionv1alpha1.AgentRunPhaseFailed {
			r.projectLifecycle(run, observation)
			return ctrl.Result{}, nil
		}
	}

	started := cancellationStartedAt(run)
	if started.IsZero() {
		started = r.now()
	}
	forced := !r.now().Before(started.Add(cancellationDeadline))
	r.projectCancelling(run, forced)

	if forced {
		if err := r.forceDeleteExecution(ctx, job); err != nil {
			return ctrl.Result{}, r.recordCancellationError(run, "ForcedCancellationFailed", err)
		}
	} else if job.DeletionTimestamp.IsZero() {
		propagation := metav1.DeletePropagationForeground
		if err := r.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.recordCancellationError(run, "GracefulCancellationFailed", err)
		}
	}
	return ctrl.Result{RequeueAfter: cancellationRequeue}, nil
}

func (r *AgentRunReconciler) projectCancelling(run *executionv1alpha1.AgentRun, forced bool) {
	run.Status.Phase = executionv1alpha1.AgentRunPhaseCancelling
	reason := "GracefulTerminationPending"
	message := "Execution Job is terminating within the cancellation deadline"
	if forced {
		reason = "ForcedTerminationPending"
		message = "Execution Job exceeded the cancellation deadline and is being forcibly terminated"
	}
	r.setCondition(run, ConditionCancellationComplete, metav1.ConditionFalse, reason, message)
	r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "CancellationInProgress", "Execution Job is being terminated")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "CancellationInProgress", "Cancellation is not yet complete")
	r.upsertAttemptStatus(run)
}

func (r *AgentRunReconciler) completeCancellation(run *executionv1alpha1.AgentRun, forced bool) {
	run.Status.Phase = executionv1alpha1.AgentRunPhaseCancelled
	if run.Status.CompletionTime == nil {
		completion := metav1.NewTime(r.now())
		run.Status.CompletionTime = &completion
	}
	reason := "GracefulCancellation"
	message := "Execution cancellation completed gracefully"
	if forced {
		reason = "ForcedCancellation"
		message = "Execution cancellation completed after forced termination"
	}
	r.setCondition(run, ConditionCancellationComplete, metav1.ConditionTrue, reason, message)
	r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "ExecutionCancelled", "Execution Job is absent after cancellation")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "ExecutionCancelled", "Execution was cancelled")
	r.upsertAttemptStatus(run)
}

func (r *AgentRunReconciler) forceDeleteExecution(ctx context.Context, job *batchv1.Job) error {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(job.Namespace), client.MatchingLabels{batchv1.JobNameLabel: job.Name}); err != nil {
		return err
	}
	zero := int64(0)
	background := metav1.DeletePropagationBackground
	for index := range pods.Items {
		owner := metav1.GetControllerOf(&pods.Items[index])
		if owner == nil || owner.APIVersion != batchv1.SchemeGroupVersion.String() || owner.Kind != "Job" || owner.Name != job.Name || owner.UID != job.UID {
			continue
		}
		if err := r.Delete(ctx, &pods.Items[index], &client.DeleteOptions{GracePeriodSeconds: &zero, PropagationPolicy: &background}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	foreground := metav1.DeletePropagationForeground
	if err := r.Delete(ctx, job, &client.DeleteOptions{GracePeriodSeconds: &zero, PropagationPolicy: &foreground}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *AgentRunReconciler) recordCancellationError(run *executionv1alpha1.AgentRun, reason string, err error) error {
	reconcileErr := classifyAPIError(reason, err)
	var existing *ReconcileError
	if errors.As(err, &existing) {
		reconcileErr = existing
	}
	run.Status.Phase = executionv1alpha1.AgentRunPhaseCancelling
	run.Status.FailureCategory = executionv1alpha1.FailureCategoryTransientDependency
	if reconcileErr.Class == ErrorClassPermanent {
		run.Status.FailureCategory = executionv1alpha1.FailureCategoryPolicy
	}
	if reason == "CancellationJobConflict" || reason == "CancellationIdentityInvalid" {
		run.Status.FailureCategory = executionv1alpha1.FailureCategoryInternal
	}
	run.Status.FailureReason = "Cancellation reconciliation could not make progress"
	r.setCondition(run, ConditionCancellationComplete, metav1.ConditionFalse, reason, "Cancellation reconciliation could not make progress")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, reason, "Cancellation reconciliation could not make progress")
	r.upsertAttemptStatus(run)
	return reconcileErr
}

func cancellationStartedAt(run *executionv1alpha1.AgentRun) time.Time {
	for index := range run.Status.Conditions {
		condition := &run.Status.Conditions[index]
		if condition.Type == ConditionCancellationComplete && condition.Status == metav1.ConditionFalse {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

func cancellationWasForced(run *executionv1alpha1.AgentRun, now time.Time) bool {
	for index := range run.Status.Conditions {
		condition := &run.Status.Conditions[index]
		if condition.Type == ConditionCancellationComplete {
			return condition.Reason == "ForcedTerminationPending" || condition.Reason == "ForcedCancellation" ||
				(condition.Status == metav1.ConditionFalse && !now.Before(condition.LastTransitionTime.Add(cancellationDeadline)))
		}
	}
	return false
}
