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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
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
	"github.com/sid995/agentforge/operator/internal/naming"
	"github.com/sid995/agentforge/operator/internal/resources"
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
	reconciler := &AgentRunReconciler{
		Client: recording, ResourceBuilder: resources.NewDefaultBuilder(),
		Now: func() time.Time { return fixedReconcileTime },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := baseClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: controllerTestNamespace}}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := baseClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "artifact-store", Namespace: controllerTestNamespace}}); err != nil {
		t.Fatalf("create artifact destination: %v", err)
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
		appliesAfterFirst := len(recording.appliedKinds)
		if writesAfterFirst == 0 {
			t.Fatal("first reconcile did not write initialized status")
		}
		if got, want := recording.appliedKinds, []string{"ServiceAccount", "ConfigMap", "PersistentVolumeClaim"}; !slices.Equal(got, want) {
			t.Fatalf("prerequisite apply order: want %v, got %v", want, got)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("duplicate reconcile: %v", err)
		}
		if recording.statusPatches != writesAfterFirst {
			t.Fatalf("duplicate reconcile wrote status: before=%d after=%d", writesAfterFirst, recording.statusPatches)
		}
		if len(recording.appliedKinds) != appliesAfterFirst {
			t.Fatalf("duplicate reconcile reapplied matching resources: before=%d after=%d", appliesAfterFirst, len(recording.appliedKinds))
		}
		stored = getControllerAgentRun(t, ctx, baseClient, run.Name)
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		serviceAccount := &corev1.ServiceAccount{}
		key := client.ObjectKey{Namespace: run.Namespace, Name: names.ServiceAccount}
		if err := baseClient.Get(ctx, key, serviceAccount); err != nil {
			t.Fatalf("get ServiceAccount: %v", err)
		}
		serviceAccount.Labels["external.example/trace"] = "preserve"
		if err := baseClient.Update(ctx, serviceAccount); err != nil {
			t.Fatalf("add external label: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile external metadata: %v", err)
		}
		if err := baseClient.Get(ctx, key, serviceAccount); err != nil {
			t.Fatalf("get ServiceAccount after reconcile: %v", err)
		}
		if serviceAccount.Labels["external.example/trace"] != "preserve" {
			t.Fatal("reconcile overwrote externally owned metadata")
		}
		if len(recording.appliedKinds) != appliesAfterFirst {
			t.Fatal("externally owned metadata caused an unnecessary apply")
		}
	})

	t.Run("bound storage unlocks network policy and Job", func(t *testing.T) {
		run := getControllerAgentRun(t, ctx, baseClient, "initialize")
		names, err := naming.ForAgentRun(run)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		workspace := &corev1.PersistentVolumeClaim{}
		key := client.ObjectKey{Namespace: run.Namespace, Name: names.Workspace}
		if err := baseClient.Get(ctx, key, workspace); err != nil {
			t.Fatalf("get workspace: %v", err)
		}
		workspace.Status.Phase = corev1.ClaimBound
		if err := baseClient.Status().Update(ctx, workspace); err != nil {
			t.Fatalf("bind workspace: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile bound workspace: %v", err)
		}
		for _, object := range []client.Object{
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: names.NetworkPolicy, Namespace: run.Namespace}},
			&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: names.Job, Namespace: run.Namespace}},
		} {
			if err := baseClient.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
				t.Fatalf("get %T: %v", object, err)
			}
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.JobName != names.Job || stored.Status.Phase != executionv1alpha1.AgentRunPhaseProvisioning {
			t.Fatalf("unexpected provisioned status: %#v", stored.Status)
		}
		for _, conditionType := range []string{
			ConditionServiceAccountReady, ConditionConfigurationReady, ConditionWorkspaceReady,
			ConditionNetworkPolicyReady, ConditionJobReady,
		} {
			condition := meta.FindStatusCondition(stored.Status.Conditions, conditionType)
			if condition == nil || condition.Status != metav1.ConditionTrue {
				t.Fatalf("condition %q is not ready: %#v", conditionType, condition)
			}
		}
	})

	t.Run("observed Job and Pod state projects running and successful attempts", func(t *testing.T) {
		run := getControllerAgentRun(t, ctx, baseClient, "initialize")
		names, err := naming.ForAgentRun(run)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		job := &batchv1.Job{}
		jobKey := client.ObjectKey{Namespace: run.Namespace, Name: names.Job}
		if err := baseClient.Get(ctx, jobKey, job); err != nil {
			t.Fatalf("get Job: %v", err)
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.Job + "-pod", Namespace: run.Namespace,
				Labels:          map[string]string{batchv1.JobNameLabel: job.Name},
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers:    []corev1.Container{{Name: "runner", Image: "example.invalid/runner:test"}},
			},
		}
		if err := baseClient.Create(ctx, pod); err != nil {
			t.Fatalf("create Pod: %v", err)
		}
		started := metav1.NewTime(fixedReconcileTime.Add(time.Minute))
		pod.Status.Phase = corev1.PodRunning
		pod.Status.StartTime = &started
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "runner", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: started}},
		}}
		if err := baseClient.Status().Update(ctx, pod); err != nil {
			t.Fatalf("set running Pod status: %v", err)
		}
		job.Status.Active = 1
		job.Status.StartTime = &started
		if err := baseClient.Status().Update(ctx, job); err != nil {
			t.Fatalf("set active Job status: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile running Job: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.Phase != executionv1alpha1.AgentRunPhaseRunning || stored.Status.PodName != pod.Name || len(stored.Status.Attempts) != 1 {
			t.Fatalf("running lifecycle was not projected: %#v", stored.Status)
		}

		completion := metav1.NewTime(started.Add(2 * time.Minute))
		if err := baseClient.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
			t.Fatalf("refresh Pod: %v", err)
		}
		pod.Status.Phase = corev1.PodSucceeded
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "runner", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 0, Reason: "Completed", FinishedAt: completion,
				Message: `{"schemaVersion":1,"artifactManifestRef":"s3://agentforge/results/initialize.json"}`,
			}},
		}}
		if err := baseClient.Status().Update(ctx, pod); err != nil {
			t.Fatalf("set completed Pod status: %v", err)
		}
		if err := baseClient.Get(ctx, jobKey, job); err != nil {
			t.Fatalf("refresh Job: %v", err)
		}
		job.Status.Active = 0
		job.Status.Succeeded = 1
		job.Status.CompletionTime = &completion
		job.Status.Conditions = []batchv1.JobCondition{
			{Type: batchv1.JobSuccessCriteriaMet, Status: corev1.ConditionTrue, Reason: "CompletionsReached", LastTransitionTime: completion},
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, Reason: "CompletionsReached", LastTransitionTime: completion},
		}
		if err := baseClient.Status().Update(ctx, job); err != nil {
			t.Fatalf("set completed Job status: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile completed Job: %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.Phase != executionv1alpha1.AgentRunPhaseSucceeded ||
			stored.Status.ArtifactManifestRef != "s3://agentforge/results/initialize.json" ||
			stored.Status.CompletionTime == nil || stored.Status.Attempts[0].Phase != executionv1alpha1.AgentRunPhaseSucceeded {
			t.Fatalf("successful lifecycle was not projected: %#v", stored.Status)
		}
		writes := recording.statusPatches
		applies := len(recording.appliedKinds)
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("repeat terminal reconcile: %v", err)
		}
		if recording.statusPatches != writes || len(recording.appliedKinds) != applies {
			t.Fatal("terminal reconcile was not a stable no-op")
		}
	})

	t.Run("externally deleted observed Job fails without recreation", func(t *testing.T) {
		run := validControllerAgentRun("deleted-job")
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("create prerequisites: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		workspace := &corev1.PersistentVolumeClaim{}
		if err := baseClient.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: names.Workspace}, workspace); err != nil {
			t.Fatalf("get workspace: %v", err)
		}
		workspace.Status.Phase = corev1.ClaimBound
		if err := baseClient.Status().Update(ctx, workspace); err != nil {
			t.Fatalf("bind workspace: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("create Job: %v", err)
		}
		job := &batchv1.Job{}
		jobKey := client.ObjectKey{Namespace: run.Namespace, Name: names.Job}
		if err := baseClient.Get(ctx, jobKey, job); err != nil {
			t.Fatalf("get Job: %v", err)
		}
		if err := baseClient.Delete(ctx, job); err != nil {
			t.Fatalf("delete Job: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile deleted Job: %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.Phase != executionv1alpha1.AgentRunPhaseFailed || stored.Status.FailureCategory != executionv1alpha1.FailureCategoryInternal ||
			len(stored.Status.Attempts) != 1 || stored.Status.Attempts[0].FailureReason == "" {
			t.Fatalf("missing Job was not recorded: %#v", stored.Status)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("repeat missing Job reconcile: %v", err)
		}
		if err := baseClient.Get(ctx, jobKey, job); err == nil && job.DeletionTimestamp.IsZero() {
			t.Fatal("externally deleted Job was recreated")
		} else if err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("get deleted Job: %v", err)
		}
	})

	t.Run("missing configuration reference requeues without later resources", func(t *testing.T) {
		run := validControllerAgentRun("missing-reference")
		run.Spec.ArtifactDestinationRef.Name = "missing-artifact-store"
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		result, err := reconciler.Reconcile(ctx, requestFor(run.Name))
		if err != nil || result.RequeueAfter != prerequisiteRequeue {
			t.Fatalf("missing-reference reconcile: result=%#v error=%v", result, err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		condition := meta.FindStatusCondition(stored.Status.Conditions, ConditionConfigurationReady)
		if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ReferenceNotFound" {
			t.Fatalf("unexpected configuration condition: %#v", condition)
		}
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		for _, object := range []client.Object{
			&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: names.Workspace, Namespace: run.Namespace}},
			&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: names.NetworkPolicy, Namespace: run.Namespace}},
			&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: names.Job, Namespace: run.Namespace}},
		} {
			if getErr := baseClient.Get(ctx, client.ObjectKeyFromObject(object), object); !apierrors.IsNotFound(getErr) {
				t.Fatalf("expected %T to be absent, got %v", object, getErr)
			}
		}
	})

	t.Run("missing Secret reference is reported", func(t *testing.T) {
		run := validControllerAgentRun("missing-secret")
		run.Spec.SecretRefs = []executionv1alpha1.LocalObjectReference{{Name: "missing-credential"}}
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		result, err := reconciler.Reconcile(ctx, requestFor(run.Name))
		if err != nil || result.RequeueAfter != prerequisiteRequeue {
			t.Fatalf("missing-secret reconcile: result=%#v error=%v", result, err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		condition := meta.FindStatusCondition(stored.Status.Conditions, ConditionConfigurationReady)
		if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ReferenceNotFound" ||
			condition.Message != "Required Secret \"missing-credential\" is not available" {
			t.Fatalf("unexpected Secret condition: %#v", condition)
		}
	})

	t.Run("wait-for-first-consumer storage permits Job scheduling", func(t *testing.T) {
		bindingMode := storagev1.VolumeBindingWaitForFirstConsumer
		storageClass := &storagev1.StorageClass{
			ObjectMeta:        metav1.ObjectMeta{Name: "wait-consumer"},
			Provisioner:       "example.test/provisioner",
			VolumeBindingMode: &bindingMode,
		}
		if err := baseClient.Create(ctx, storageClass); err != nil && !apierrors.IsAlreadyExists(err) {
			t.Fatalf("create StorageClass: %v", err)
		}
		run := validControllerAgentRun("wait-consumer")
		run.Spec.Workspace.StorageClassName = storageClass.Name
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile first-consumer storage: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		condition := meta.FindStatusCondition(stored.Status.Conditions, ConditionWorkspaceReady)
		if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "FirstConsumerPending" {
			t.Fatalf("unexpected workspace condition: %#v", condition)
		}
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		job := &batchv1.Job{}
		if err := baseClient.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: names.Job}, job); err != nil {
			t.Fatalf("first-consumer Job was not created: %v", err)
		}
	})

	t.Run("conflicting pre-existing resource is permanent", func(t *testing.T) {
		run := validControllerAgentRun("resource-conflict")
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create AgentRun: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive names: %v", err)
		}
		if err := baseClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: names.ServiceAccount, Namespace: run.Namespace}}); err != nil {
			t.Fatalf("create conflicting ServiceAccount: %v", err)
		}
		_, err = reconciler.Reconcile(ctx, requestFor(run.Name))
		var reconcileErr *ReconcileError
		if !errors.As(err, &reconcileErr) || reconcileErr.Class != ErrorClassPermanent || reconcileErr.Reason != "ServiceAccountReconcileFailed" {
			t.Fatalf("expected permanent ServiceAccount conflict, got %v", err)
		}
		stored = getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.FailureCategory != executionv1alpha1.FailureCategoryConflict {
			t.Fatalf("unexpected conflict status: %#v", stored.Status)
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

	t.Run("cancellation before Job creation creates no work", func(t *testing.T) {
		run := validControllerAgentRun("cancel-before-creation")
		run.Spec.DesiredState = executionv1alpha1.DesiredStateCancelled
		if err := baseClient.Create(ctx, run); err != nil {
			t.Fatalf("create cancelled AgentRun: %v", err)
		}
		if _, err := reconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("reconcile cancellation before creation: %v", err)
		}
		stored := getControllerAgentRun(t, ctx, baseClient, run.Name)
		if stored.Status.Phase != executionv1alpha1.AgentRunPhaseCancelled {
			t.Fatalf("cancellation before Job creation did not complete: %#v", stored.Status)
		}
		names, err := naming.ForAgentRun(stored)
		if err != nil {
			t.Fatalf("derive cancellation names: %v", err)
		}
		if err := baseClient.Get(ctx, client.ObjectKey{Namespace: stored.Namespace, Name: names.Job}, &batchv1.Job{}); !apierrors.IsNotFound(err) {
			t.Fatalf("cancellation created an execution Job: %v", err)
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
		conflictReconciler := &AgentRunReconciler{
			Client: conflicting, ResourceBuilder: resources.NewDefaultBuilder(),
			Now: func() time.Time { return fixedReconcileTime },
		}
		if _, err := conflictReconciler.Reconcile(ctx, requestFor(run.Name)); !apierrors.IsConflict(err) {
			t.Fatalf("expected transient status conflict, got %v", err)
		}
		if _, err := conflictReconciler.Reconcile(ctx, requestFor(run.Name)); err != nil {
			t.Fatalf("retry after status conflict: %v", err)
		}
		assertInitializedStatus(t, getControllerAgentRun(t, ctx, baseClient, run.Name))
	})

}

func TestInvalidSpecIsPermanentAndRecorded(t *testing.T) {
	scheme := controllerTestScheme(t)
	run := validControllerAgentRun("invalid")
	run.Generation = 1
	run.Spec.RunID = "invalid"
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&executionv1alpha1.AgentRun{}).WithObjects(run).Build()
	reconciler := &AgentRunReconciler{
		Client: fakeClient, ResourceBuilder: resources.NewDefaultBuilder(),
		Now: func() time.Time { return fixedReconcileTime },
	}

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
	first, err := naming.ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive names: %v", err)
	}
	second, err := naming.ForAgentRun(run.DeepCopy())
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
	nextAttempt, err := naming.ForAgentRun(run)
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
		"storage":  storagev1.AddToScheme,
		"AgentRun": executionv1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("register %s scheme: %v", name, err)
		}
	}
	return scheme
}

func validControllerAgentRun(name string) *executionv1alpha1.AgentRun {
	digest := sha256.Sum256([]byte(name))
	runSuffix := hex.EncodeToString(digest[:6])
	attemptSuffix := hex.EncodeToString(digest[6:12])
	return &executionv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: controllerTestNamespace},
		Spec: executionv1alpha1.AgentRunSpec{
			TenantID:         "019b0000-0000-7000-8000-000000000002",
			ProjectID:        "019b0000-0000-7000-8000-000000000003",
			RunID:            executionv1alpha1.UUIDv7("019b0000-0000-7000-8000-" + runSuffix),
			AttemptID:        executionv1alpha1.UUIDv7("019b0000-0000-7000-8000-" + attemptSuffix),
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
	if run.Status.Phase != executionv1alpha1.AgentRunPhaseProvisioning || run.Status.Attempt != run.Spec.Attempt ||
		run.Status.Namespace != run.Namespace || run.Status.ObservedGeneration != run.Generation {
		t.Fatalf("unexpected initialized status: %#v", run.Status)
	}
	specValid := meta.FindStatusCondition(run.Status.Conditions, ConditionSpecValid)
	ready := meta.FindStatusCondition(run.Status.Conditions, ConditionReady)
	if specValid == nil || specValid.Status != metav1.ConditionTrue || specValid.Reason != "Accepted" {
		t.Fatalf("unexpected SpecValid condition: %#v", specValid)
	}
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "StoragePending" {
		t.Fatalf("unexpected Ready condition: %#v", ready)
	}
	if !specValid.LastTransitionTime.Equal(&metav1.Time{Time: fixedReconcileTime}) {
		t.Fatalf("condition did not use injected clock: %s", specValid.LastTransitionTime)
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
	appliedKinds    []string
}

func (c *recordingClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	if _, ok := object.(*executionv1alpha1.AgentRun); ok {
		c.metadataPatches++
	} else if patch.Type() == types.ApplyPatchType {
		c.appliedKinds = append(c.appliedKinds, resourceKind(object))
	}
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
