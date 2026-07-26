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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controlleroptions "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
	"github.com/sid995/agentforge/operator/internal/resources"
)

const (
	// RetainedResourcesFinalizer gates cleanup only when the desired workspace is explicitly retained.
	RetainedResourcesFinalizer = "execution.agentforge.dev/retained-resources"

	ConditionSpecValid            = "SpecValid"
	ConditionServiceAccountReady  = "ServiceAccountReady"
	ConditionConfigurationReady   = "ConfigurationReady"
	ConditionWorkspaceReady       = "WorkspaceReady"
	ConditionNetworkPolicyReady   = "NetworkPolicyReady"
	ConditionJobReady             = "JobReady"
	ConditionReady                = "Ready"
	ConditionCancellationComplete = "CancellationComplete"
	ConditionRetryReady           = "RetryReady"
	ConditionCleanupPending       = "CleanupPending"

	minimumRetryDelay = time.Second
	maximumRetryDelay = 2 * time.Minute
)

// AgentRunReconciler projects durable AgentRun intent into namespaced execution resources.
type AgentRunReconciler struct {
	client.Client
	APIReader       client.Reader
	ResourceBuilder *resources.Builder
	Now             func() time.Time
}

// +kubebuilder:rbac:groups=execution.agentforge.dev,resources=agentruns,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=execution.agentforge.dev,resources=agentruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=execution.agentforge.dev,resources=agentruns/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts;configmaps;persistentvolumeclaims,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;delete

// Reconcile fetches an AgentRun, validates it, establishes cleanup ownership, and ensures prerequisites.
func (r *AgentRunReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	started := time.Now()
	outcome := "success"
	defer func() {
		agentRunReconciliations.WithLabelValues(outcome).Inc()
		agentRunReconcileDuration.Observe(time.Since(started).Seconds())
	}()

	run := &executionv1alpha1.AgentRun{}
	if err := r.Get(ctx, request.NamespacedName, run); err != nil {
		if apierrors.IsNotFound(err) {
			outcome = "not_found"
			return ctrl.Result{}, nil
		}
		reconcileErr := classifyAPIError("FetchFailed", err)
		outcome = string(reconcileErr.Class) + "_error"
		return ctrl.Result{}, terminalIfPermanent(reconcileErr)
	}

	log := ctrl.LoggerFrom(ctx).WithValues(
		"tenant_id", run.Spec.TenantID,
		"project_id", run.Spec.ProjectID,
		"run_id", run.Spec.RunID,
		"attempt", naming.CurrentAttempt(run),
	)
	ctx = ctrl.LoggerInto(ctx, log)

	if !run.DeletionTimestamp.IsZero() {
		outcome = "deleting"
		if !controllerutil.ContainsFinalizer(run, RetainedResourcesFinalizer) {
			return ctrl.Result{}, nil
		}
		beforeStatus := run.DeepCopy()
		cleanupComplete, cleanupErr := r.reconcileCleanup(ctx, run)
		if _, err := r.patchStatusIfChanged(ctx, beforeStatus, run); err != nil {
			reconcileErr := classifyAPIError("DeletionStatusWriteFailed", err)
			outcome = "cleanup_pending"
			log.Info("Retained AgentRun cleanup status is waiting for operator retry", "reason", reconcileErr.Reason)
			return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
		}
		if cleanupErr != nil {
			outcome = "cleanup_pending"
			log.Info("Retained AgentRun cleanup is waiting for operator retry", "reason", cleanupErr.Reason)
			return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
		}
		if !cleanupComplete {
			return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
		}
		beforeMetadata := run.DeepCopy()
		controllerutil.RemoveFinalizer(run, RetainedResourcesFinalizer)
		patch := client.MergeFromWithOptions(beforeMetadata, client.MergeFromWithOptimisticLock{})
		if err := r.Patch(ctx, run, patch); err != nil {
			reconcileErr := classifyAPIError("FinalizerRemovalFailed", err)
			beforeFailureStatus := run.DeepCopy()
			r.setCondition(run, ConditionCleanupPending, metav1.ConditionTrue, "FinalizerRemovalFailed", "Cleanup completed but finalizer removal could not make progress")
			_, _ = r.patchStatusIfChanged(ctx, beforeFailureStatus, run)
			outcome = "cleanup_pending"
			log.Info("Retained AgentRun finalizer removal is waiting for operator retry", "reason", reconcileErr.Reason)
			return ctrl.Result{RequeueAfter: cleanupRequeue}, nil
		}
		if condition := meta.FindStatusCondition(run.Status.Conditions, ConditionCleanupPending); condition != nil && condition.Reason == "CleanupEscalated" {
			log.Info("Cleanup deadline elapsed; released finalizer for retained-resource operator review")
		} else {
			log.Info("Completed retained-resource cleanup and removed finalizer")
		}
		return ctrl.Result{}, nil
	}

	if err := validateAgentRun(run); err != nil {
		beforeStatus := run.DeepCopy()
		r.initializeInvalidStatus(run)
		if _, statusErr := r.patchStatusIfChanged(ctx, beforeStatus, run); statusErr != nil {
			reconcileErr := classifyAPIError("InvalidSpecStatusWriteFailed", statusErr)
			outcome = string(reconcileErr.Class) + "_error"
			return ctrl.Result{}, terminalIfPermanent(reconcileErr)
		}
		outcome = "permanent_error"
		return ctrl.Result{}, reconcile.TerminalError(newReconcileError(ErrorClassPermanent, "InvalidSpec", err))
	}

	if run.Spec.Workspace.RetentionPolicy == executionv1alpha1.WorkspaceRetentionRetain &&
		!controllerutil.ContainsFinalizer(run, RetainedResourcesFinalizer) {
		beforeMetadata := run.DeepCopy()
		controllerutil.AddFinalizer(run, RetainedResourcesFinalizer)
		patch := client.MergeFromWithOptions(beforeMetadata, client.MergeFromWithOptimisticLock{})
		if err := r.Patch(ctx, run, patch); err != nil {
			reconcileErr := classifyAPIError("FinalizerWriteFailed", err)
			outcome = string(reconcileErr.Class) + "_error"
			return ctrl.Result{}, terminalIfPermanent(reconcileErr)
		}
		log.Info("Added retained-resource finalizer")
	}

	beforeStatus := run.DeepCopy()
	if currentAttemptTerminal(run) {
		if run.Spec.DesiredState == executionv1alpha1.DesiredStateRunning && retryAllowed(run) {
			result := r.reconcileRetry(run)
			changed, err := r.patchStatusIfChanged(ctx, beforeStatus, run)
			if err != nil {
				reconcileErr := classifyAPIError("RetryStatusWriteFailed", err)
				outcome = string(reconcileErr.Class) + "_error"
				return ctrl.Result{}, terminalIfPermanent(reconcileErr)
			}
			if !changed {
				outcome = "unchanged"
			}
			return result, nil
		}
		run.Status.ObservedGeneration = run.Generation
		for index := range run.Status.Conditions {
			run.Status.Conditions[index].ObservedGeneration = run.Generation
		}
		r.setCondition(run, ConditionSpecValid, metav1.ConditionTrue, "Accepted", "AgentRun desired state is valid")
		changed, err := r.patchStatusIfChanged(ctx, beforeStatus, run)
		if err != nil {
			reconcileErr := classifyAPIError("TerminalStatusWriteFailed", err)
			outcome = string(reconcileErr.Class) + "_error"
			return ctrl.Result{}, terminalIfPermanent(reconcileErr)
		}
		if !changed {
			outcome = "unchanged"
		}
		return ctrl.Result{}, nil
	}
	if run.Spec.DesiredState == executionv1alpha1.DesiredStateCancelled {
		r.initializeCancellationStatus(run)
		result, cancellationErr := r.reconcileCancellation(ctx, run)
		changed, err := r.patchStatusIfChanged(ctx, beforeStatus, run)
		if err != nil {
			reconcileErr := classifyAPIError("CancellationStatusWriteFailed", err)
			outcome = string(reconcileErr.Class) + "_error"
			return ctrl.Result{}, terminalIfPermanent(reconcileErr)
		}
		if cancellationErr != nil {
			outcome = string(errorClass(cancellationErr)) + "_error"
			return result, terminalIfPermanent(cancellationErr)
		}
		if !changed {
			outcome = "unchanged"
		}
		return result, nil
	}
	r.initializeAcceptedStatus(run)
	result, prerequisiteErr := r.reconcilePrerequisites(ctx, run)
	if prerequisiteErr == nil && retryAllowed(run) {
		result = r.reconcileRetry(run)
	}
	changed, err := r.patchStatusIfChanged(ctx, beforeStatus, run)
	if err != nil {
		reconcileErr := classifyAPIError("StatusWriteFailed", err)
		outcome = string(reconcileErr.Class) + "_error"
		return ctrl.Result{}, terminalIfPermanent(reconcileErr)
	}
	if prerequisiteErr != nil {
		outcome = string(errorClass(prerequisiteErr)) + "_error"
		return result, terminalIfPermanent(prerequisiteErr)
	}
	if changed {
		log.Info("Updated AgentRun reconciliation status", "observed_generation", run.Status.ObservedGeneration)
	} else {
		outcome = "unchanged"
	}
	return result, nil
}

func (r *AgentRunReconciler) initializeCancellationStatus(run *executionv1alpha1.AgentRun) {
	if run.Status.Phase == "" {
		run.Status.Phase = executionv1alpha1.AgentRunPhasePending
	}
	if run.Status.Attempt == 0 {
		run.Status.Attempt = run.Spec.Attempt
	}
	if run.Status.Namespace == "" {
		run.Status.Namespace = run.Namespace
	}
	run.Status.ObservedGeneration = run.Generation
	r.setCondition(run, ConditionSpecValid, metav1.ConditionTrue, "Accepted", "AgentRun desired state is valid")
}

func (r *AgentRunReconciler) initializeAcceptedStatus(run *executionv1alpha1.AgentRun) {
	if run.Status.Phase == "" {
		run.Status.Phase = executionv1alpha1.AgentRunPhasePending
	}
	if run.Status.Attempt == 0 {
		run.Status.Attempt = run.Spec.Attempt
	}
	if run.Status.Namespace == "" {
		run.Status.Namespace = run.Namespace
	}
	run.Status.ObservedGeneration = run.Generation
	run.Status.FailureCategory = ""
	run.Status.FailureReason = ""
	r.setCondition(run, ConditionSpecValid, metav1.ConditionTrue, "Accepted", "AgentRun desired state is valid")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "ReconciliationPending", "Execution resources have not been reconciled")
	for _, conditionType := range []string{
		ConditionServiceAccountReady,
		ConditionConfigurationReady,
		ConditionWorkspaceReady,
		ConditionNetworkPolicyReady,
		ConditionJobReady,
	} {
		if meta.FindStatusCondition(run.Status.Conditions, conditionType) == nil {
			r.setCondition(run, conditionType, metav1.ConditionFalse, "ReconciliationPending", "Prerequisite has not been reconciled")
		}
	}
}

func (r *AgentRunReconciler) initializeInvalidStatus(run *executionv1alpha1.AgentRun) {
	run.Status.Phase = executionv1alpha1.AgentRunPhaseFailed
	run.Status.ObservedGeneration = run.Generation
	run.Status.Attempt = run.Spec.Attempt
	run.Status.Namespace = run.Namespace
	run.Status.FailureCategory = executionv1alpha1.FailureCategoryValidation
	run.Status.FailureReason = "AgentRun desired state failed controller validation"
	r.setCondition(run, ConditionSpecValid, metav1.ConditionFalse, "InvalidSpec", "AgentRun desired state is invalid")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "InvalidSpec", "Execution resources will not be reconciled")
}

func (r *AgentRunReconciler) initializeDeletingStatus(run *executionv1alpha1.AgentRun) {
	run.Status.ObservedGeneration = run.Generation
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "Deleting", "AgentRun deletion is in progress")
	if meta.FindStatusCondition(run.Status.Conditions, ConditionCleanupPending) == nil {
		r.setCondition(run, ConditionCleanupPending, metav1.ConditionTrue, "CleanupStarted", "Retained workspace handoff is in progress")
	}
}

func (r *AgentRunReconciler) setCondition(run *executionv1alpha1.AgentRun, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&run.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: run.Generation,
		LastTransitionTime: metav1.NewTime(r.now()),
		Reason:             reason,
		Message:            message,
	})
}

func (r *AgentRunReconciler) patchStatusIfChanged(ctx context.Context, before, after *executionv1alpha1.AgentRun) (bool, error) {
	if equality.Semantic.DeepEqual(before.Status, after.Status) {
		agentRunStatusWrites.WithLabelValues("unchanged").Inc()
		return false, nil
	}
	patch := client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})
	if err := r.Status().Patch(ctx, after, patch); err != nil {
		if apierrors.IsConflict(err) {
			agentRunStatusWrites.WithLabelValues("conflict").Inc()
		} else {
			agentRunStatusWrites.WithLabelValues("error").Inc()
		}
		return false, err
	}
	agentRunStatusWrites.WithLabelValues("updated").Inc()
	return true, nil
}

func (r *AgentRunReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func terminalIfPermanent(err error) error {
	if errorClass(err) == ErrorClassPermanent {
		return reconcile.TerminalError(err)
	}
	return err
}

func newRateLimiter() workqueue.TypedRateLimiter[reconcile.Request] {
	return workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](minimumRetryDelay, maximumRetryDelay)
}

func reconciliationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(update event.UpdateEvent) bool {
			if update.ObjectOld == nil || update.ObjectNew == nil {
				return false
			}
			generationChanged := update.ObjectOld.GetGeneration() != update.ObjectNew.GetGeneration()
			deletionStarted := update.ObjectOld.GetDeletionTimestamp().IsZero() && !update.ObjectNew.GetDeletionTimestamp().IsZero()
			return generationChanged || deletionStarted
		},
	}
}

// SetupWithManager registers the AgentRun watch with bounded retry behavior.
func (r *AgentRunReconciler) SetupWithManager(manager ctrl.Manager) error {
	if r.Client == nil {
		return errors.New("AgentRun reconciler client is required")
	}
	if r.ResourceBuilder == nil {
		return errors.New("AgentRun reconciler resource builder is required")
	}
	return ctrl.NewControllerManagedBy(manager).
		For(&executionv1alpha1.AgentRun{}, builder.WithPredicates(reconciliationPredicate())).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&batchv1.Job{}).
		WithOptions(controlleroptions.Options{
			MaxConcurrentReconciles: 4,
			RateLimiter:             newRateLimiter(),
			ReconciliationTimeout:   30 * time.Second,
		}).
		Named("agentrun").
		Complete(r)
}
