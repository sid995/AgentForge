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
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/resources"
)

const (
	prerequisiteFieldOwner = "agentforge-operator"
	prerequisiteRequeue    = 5 * time.Second
)

type resourceConflictError struct {
	kind string
	name string
	err  error
}

func (e *resourceConflictError) Error() string {
	return fmt.Sprintf("conflicting %s %q: %v", e.kind, e.name, e.err)
}

func (e *resourceConflictError) Unwrap() error {
	return e.err
}

func (r *AgentRunReconciler) reconcilePrerequisites(ctx context.Context, run *executionv1alpha1.AgentRun) (ctrl.Result, error) {
	if r.ResourceBuilder == nil {
		err := newReconcileError(ErrorClassPermanent, "ResourceBuilderMissing", errors.New("trusted resource builder is not configured"))
		r.recordPrerequisiteFailure(run, ConditionConfigurationReady, err, executionv1alpha1.FailureCategoryInternal)
		return ctrl.Result{}, err
	}

	desired, err := r.ResourceBuilder.BuildAll(run)
	if err != nil {
		reconcileErr := newReconcileError(ErrorClassPermanent, "ResourceBuildFailed", err)
		r.recordPrerequisiteFailure(run, ConditionConfigurationReady, reconcileErr, executionv1alpha1.FailureCategoryPolicy)
		return ctrl.Result{}, reconcileErr
	}

	if err := r.ensureObject(ctx, run, desired.ServiceAccount); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionServiceAccountReady, "ServiceAccountReconcileFailed", err)
	}
	r.recordPrerequisiteReady(run, ConditionServiceAccountReady, "ServiceAccountReady", "Workload ServiceAccount is ready")

	referencesReady, err := r.configurationReferencesReady(ctx, run)
	if err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionConfigurationReady, "ConfigurationReadFailed", err)
	}
	if !referencesReady {
		run.Status.Phase = executionv1alpha1.AgentRunPhaseProvisioning
		return ctrl.Result{RequeueAfter: prerequisiteRequeue}, nil
	}
	if err := r.ensureObject(ctx, run, desired.Configuration); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionConfigurationReady, "ConfigurationReconcileFailed", err)
	}
	r.recordPrerequisiteReady(run, ConditionConfigurationReady, "ConfigurationReady", "Execution configuration references are ready")

	if err := r.ensureObject(ctx, run, desired.Workspace); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionWorkspaceReady, "WorkspaceReconcileFailed", err)
	}
	workspace := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(desired.Workspace), workspace); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionWorkspaceReady, "WorkspaceReadFailed", err)
	}
	if workspace.Status.Phase != corev1.ClaimBound {
		run.Status.Phase = executionv1alpha1.AgentRunPhaseProvisioning
		waitForConsumer, err := r.workspaceWaitsForFirstConsumer(ctx, workspace)
		if err != nil {
			return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionWorkspaceReady, "StorageClassReadFailed", err)
		}
		if !waitForConsumer {
			r.setCondition(run, ConditionWorkspaceReady, metav1.ConditionFalse, "StoragePending", "Workspace storage is not bound")
			r.setCondition(run, ConditionNetworkPolicyReady, metav1.ConditionFalse, "WorkspacePending", "Network policy waits for workspace storage")
			r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "WorkspacePending", "Execution Job waits for workspace storage")
			r.setCondition(run, ConditionReady, metav1.ConditionFalse, "StoragePending", "Execution is waiting for workspace storage")
			return ctrl.Result{RequeueAfter: prerequisiteRequeue}, nil
		}
		r.setCondition(run, ConditionWorkspaceReady, metav1.ConditionFalse, "FirstConsumerPending", "Workspace binding requires an execution Pod consumer")
	} else {
		r.recordPrerequisiteReady(run, ConditionWorkspaceReady, "WorkspaceReady", "Workspace storage is bound")
	}

	if err := r.ensureObject(ctx, run, desired.NetworkPolicy); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionNetworkPolicyReady, "NetworkPolicyReconcileFailed", err)
	}
	r.recordPrerequisiteReady(run, ConditionNetworkPolicyReady, "NetworkPolicyReady", "Workload network policy is ready")

	if run.Status.JobName != "" {
		if run.Status.JobName != desired.Job.Name {
			reconcileErr := newReconcileError(ErrorClassPermanent, "JobIdentityConflict", errors.New("status Job identity does not match the current attempt"))
			r.recordPrerequisiteFailure(run, ConditionJobReady, reconcileErr, executionv1alpha1.FailureCategoryInternal)
			return ctrl.Result{}, reconcileErr
		}
		existingJob := &batchv1.Job{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(desired.Job), existingJob); err != nil {
			if apierrors.IsNotFound(err) {
				r.recordMissingJob(run, desired.Job.Name)
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionJobReady, "JobReadFailed", err)
		}
		if !existingJob.DeletionTimestamp.IsZero() {
			r.recordMissingJob(run, desired.Job.Name)
			return ctrl.Result{}, nil
		}
	}
	if err := r.ensureObject(ctx, run, desired.Job); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionJobReady, "JobReconcileFailed", err)
	}
	run.Status.JobName = desired.Job.Name
	r.recordPrerequisiteReady(run, ConditionJobReady, "JobReady", "Execution Job is created")
	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(desired.Job), job); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionJobReady, "JobReadFailed", err)
	}
	return r.observeJob(ctx, run, job)
}

func (r *AgentRunReconciler) workspaceWaitsForFirstConsumer(ctx context.Context, workspace *corev1.PersistentVolumeClaim) (bool, error) {
	if workspace.Spec.StorageClassName == nil || *workspace.Spec.StorageClassName == "" {
		return false, nil
	}
	storageClass := &storagev1.StorageClass{}
	if err := r.uncachedReader().Get(ctx, client.ObjectKey{Name: *workspace.Spec.StorageClassName}, storageClass); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return storageClass.VolumeBindingMode != nil && *storageClass.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer, nil
}

func (r *AgentRunReconciler) configurationReferencesReady(ctx context.Context, run *executionv1alpha1.AgentRun) (bool, error) {
	configNames := make([]string, 0, len(run.Spec.ConfigurationRefs)+1)
	configNames = append(configNames, run.Spec.ArtifactDestinationRef.Name)
	for _, reference := range run.Spec.ConfigurationRefs {
		configNames = append(configNames, reference.Name)
	}
	slices.Sort(configNames)
	configNames = slices.Compact(configNames)
	for _, name := range configNames {
		object := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: name}, object); err != nil {
			if apierrors.IsNotFound(err) {
				r.setCondition(run, ConditionConfigurationReady, metav1.ConditionFalse, "ReferenceNotFound", fmt.Sprintf("Required ConfigMap %q is not available", name))
				r.setCondition(run, ConditionWorkspaceReady, metav1.ConditionFalse, "ConfigurationPending", "Workspace waits for execution configuration")
				r.setCondition(run, ConditionNetworkPolicyReady, metav1.ConditionFalse, "ConfigurationPending", "Network policy waits for execution configuration")
				r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "ConfigurationPending", "Execution Job waits for execution configuration")
				r.setCondition(run, ConditionReady, metav1.ConditionFalse, "ConfigurationPending", "Execution configuration references are not ready")
				return false, nil
			}
			return false, err
		}
	}

	secretNames := make([]string, 0, len(run.Spec.SecretRefs))
	for _, reference := range run.Spec.SecretRefs {
		secretNames = append(secretNames, reference.Name)
	}
	slices.Sort(secretNames)
	secretNames = slices.Compact(secretNames)
	for _, name := range secretNames {
		// Metadata is enough to gate scheduling; use the uncached reader and never
		// place Secret values in the controller cache or reconciliation status.
		object := &metav1.PartialObjectMetadata{}
		object.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
		if err := r.uncachedReader().Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: name}, object); err != nil {
			if apierrors.IsNotFound(err) {
				r.setCondition(run, ConditionConfigurationReady, metav1.ConditionFalse, "ReferenceNotFound", fmt.Sprintf("Required Secret %q is not available", name))
				r.setCondition(run, ConditionWorkspaceReady, metav1.ConditionFalse, "ConfigurationPending", "Workspace waits for execution configuration")
				r.setCondition(run, ConditionNetworkPolicyReady, metav1.ConditionFalse, "ConfigurationPending", "Network policy waits for execution configuration")
				r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "ConfigurationPending", "Execution Job waits for execution configuration")
				r.setCondition(run, ConditionReady, metav1.ConditionFalse, "ConfigurationPending", "Execution configuration references are not ready")
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

func (r *AgentRunReconciler) uncachedReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func (r *AgentRunReconciler) ensureObject(ctx context.Context, run *executionv1alpha1.AgentRun, desired client.Object) error {
	existing := emptyObjectFor(desired)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if err == nil {
		if ownershipErr := validateExistingOwnership(run, desired, existing); ownershipErr != nil {
			return &resourceConflictError{kind: resourceKind(desired), name: desired.GetName(), err: ownershipErr}
		}
		if managedObjectMatches(desired, existing) {
			return nil
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	payload, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("marshal %s apply configuration: %w", resourceKind(desired), err)
	}
	applyPatch := client.RawPatch(types.ApplyPatchType, payload)
	if err := r.Patch(ctx, desired, applyPatch, client.FieldOwner(prerequisiteFieldOwner)); err != nil {
		if apierrors.IsConflict(err) {
			return &resourceConflictError{kind: resourceKind(desired), name: desired.GetName(), err: err}
		}
		return err
	}
	return nil
}

func (r *AgentRunReconciler) handlePrerequisiteError(run *executionv1alpha1.AgentRun, conditionType, reason string, err error) error {
	var conflict *resourceConflictError
	if errors.As(err, &conflict) {
		reconcileErr := newReconcileError(ErrorClassPermanent, reason, err)
		r.recordPrerequisiteFailure(run, conditionType, reconcileErr, executionv1alpha1.FailureCategoryConflict)
		return reconcileErr
	}
	reconcileErr := classifyAPIError(reason, err)
	category := executionv1alpha1.FailureCategoryTransientDependency
	if reconcileErr.Class == ErrorClassPermanent {
		category = executionv1alpha1.FailureCategoryPolicy
	}
	r.recordPrerequisiteFailure(run, conditionType, reconcileErr, category)
	return reconcileErr
}

func (r *AgentRunReconciler) recordPrerequisiteReady(run *executionv1alpha1.AgentRun, conditionType, reason, message string) {
	r.setCondition(run, conditionType, metav1.ConditionTrue, reason, message)
}

func (r *AgentRunReconciler) recordPrerequisiteFailure(run *executionv1alpha1.AgentRun, conditionType string, err *ReconcileError, category executionv1alpha1.FailureCategory) {
	if err.Class == ErrorClassPermanent {
		run.Status.Phase = executionv1alpha1.AgentRunPhaseFailed
	} else {
		run.Status.Phase = executionv1alpha1.AgentRunPhaseProvisioning
	}
	run.Status.FailureCategory = category
	run.Status.FailureReason = fmt.Sprintf("%s could not be reconciled", conditionType)
	r.setCondition(run, conditionType, metav1.ConditionFalse, err.Reason, run.Status.FailureReason)
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, err.Reason, "Execution prerequisites are not ready")
}

func emptyObjectFor(object client.Object) client.Object {
	switch object.(type) {
	case *corev1.ServiceAccount:
		return &corev1.ServiceAccount{}
	case *corev1.ConfigMap:
		return &corev1.ConfigMap{}
	case *corev1.PersistentVolumeClaim:
		return &corev1.PersistentVolumeClaim{}
	case *networkingv1.NetworkPolicy:
		return &networkingv1.NetworkPolicy{}
	case *batchv1.Job:
		return &batchv1.Job{}
	default:
		panic(fmt.Sprintf("unsupported prerequisite object %T", object))
	}
}

func validateExistingOwnership(run *executionv1alpha1.AgentRun, desired, existing client.Object) error {
	desiredController := metav1.GetControllerOf(desired)
	existingController := metav1.GetControllerOf(existing)
	if desiredController != nil {
		if existingController == nil || existingController.UID != run.UID || existingController.Name != run.Name ||
			existingController.Kind != "AgentRun" || existingController.APIVersion != executionv1alpha1.GroupVersion.String() {
			return errors.New("resource is not controlled by this AgentRun")
		}
	} else if existingController != nil {
		return errors.New("retained resource has an unexpected controller owner")
	}
	if existing.GetAnnotations()["execution.agentforge.dev/owner-uid"] != string(run.UID) {
		return errors.New("resource owner identity annotation does not match")
	}
	return nil
}

func managedObjectMatches(desired, existing client.Object) bool {
	if !metadataContains(existing, desired) {
		return false
	}
	switch wanted := desired.(type) {
	case *corev1.ServiceAccount:
		actual := existing.(*corev1.ServiceAccount)
		return equality.Semantic.DeepEqual(wanted.AutomountServiceAccountToken, actual.AutomountServiceAccountToken) &&
			equality.Semantic.DeepEqual(wanted.Secrets, actual.Secrets) &&
			equality.Semantic.DeepEqual(wanted.ImagePullSecrets, actual.ImagePullSecrets)
	case *corev1.ConfigMap:
		actual := existing.(*corev1.ConfigMap)
		return equality.Semantic.DeepEqual(wanted.Immutable, actual.Immutable) &&
			equality.Semantic.DeepEqual(wanted.Data, actual.Data) &&
			equality.Semantic.DeepEqual(wanted.BinaryData, actual.BinaryData)
	case *corev1.PersistentVolumeClaim:
		actual := existing.(*corev1.PersistentVolumeClaim)
		return equality.Semantic.DeepDerivative(wanted.Spec, actual.Spec)
	case *networkingv1.NetworkPolicy:
		actual := existing.(*networkingv1.NetworkPolicy)
		return equality.Semantic.DeepEqual(wanted.Spec, actual.Spec)
	case *batchv1.Job:
		actual := existing.(*batchv1.Job)
		if err := resources.ValidateSecurity(&resources.ExecutionResources{
			ServiceAccount: &corev1.ServiceAccount{AutomountServiceAccountToken: boolPointer(false)},
			NetworkPolicy:  &networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{}}},
			Job:            actual,
		}); err != nil {
			return false
		}
		return metadataContains(&actual.Spec.Template, &wanted.Spec.Template) && equality.Semantic.DeepDerivative(wanted.Spec, actual.Spec)
	default:
		return false
	}
}

func metadataContains(existing, desired metav1.Object) bool {
	for key, value := range desired.GetLabels() {
		if existing.GetLabels()[key] != value {
			return false
		}
	}
	for key, value := range desired.GetAnnotations() {
		if existing.GetAnnotations()[key] != value {
			return false
		}
	}
	return true
}

func resourceKind(object client.Object) string {
	switch object.(type) {
	case *corev1.ServiceAccount:
		return "ServiceAccount"
	case *corev1.ConfigMap:
		return "ConfigMap"
	case *corev1.PersistentVolumeClaim:
		return "PersistentVolumeClaim"
	case *networkingv1.NetworkPolicy:
		return "NetworkPolicy"
	case *batchv1.Job:
		return "Job"
	default:
		return fmt.Sprintf("%T", object)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
