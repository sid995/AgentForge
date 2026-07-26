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
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

func TestCancellationBeforeJobCreation(t *testing.T) {
	run := cancellationTestRun(t, "cancel-before-job")
	reconciler := &AgentRunReconciler{Client: fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).Build(), Now: func() time.Time { return fixedReconcileTime }}

	result, err := reconciler.reconcileCancellation(context.Background(), run)
	if err != nil || result.RequeueAfter != 0 {
		t.Fatalf("reconcile cancellation: result=%#v err=%v", result, err)
	}
	assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelled, "GracefulCancellation")
}

func TestCancellationWithAlreadyMissingJobConverges(t *testing.T) {
	run := cancellationTestRun(t, "cancel-missing-job")
	names, err := naming.ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive resource names: %v", err)
	}
	run.Status.JobName = names.Job
	reconciler := &AgentRunReconciler{Client: fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).Build(), Now: func() time.Time { return fixedReconcileTime }}

	if _, err := reconciler.reconcileCancellation(context.Background(), run); err != nil {
		t.Fatalf("reconcile missing Job cancellation: %v", err)
	}
	assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelled, "GracefulCancellation")
}

func TestCancellationPendingAndRunningAreGracefulAndIdempotent(t *testing.T) {
	for _, testCase := range []struct {
		name string
		pod  bool
	}{
		{name: "pending"},
		{name: "running", pod: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			run, job, pod := cancellationObjects(t, "cancel-"+testCase.name, testCase.pod)
			objects := []client.Object{job}
			if pod != nil {
				objects = append(objects, pod)
			}
			kubernetesClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(objects...).Build()
			reconciler := &AgentRunReconciler{Client: kubernetesClient, Now: func() time.Time { return fixedReconcileTime }}

			result, err := reconciler.reconcileCancellation(context.Background(), run)
			if err != nil || result.RequeueAfter != cancellationRequeue {
				t.Fatalf("begin cancellation: result=%#v err=%v", result, err)
			}
			assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelling, "GracefulTerminationPending")

			if _, err := reconciler.reconcileCancellation(context.Background(), run); err != nil {
				t.Fatalf("complete cancellation: %v", err)
			}
			assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelled, "GracefulCancellation")
			if _, err := reconciler.reconcileCancellation(context.Background(), run); err != nil {
				t.Fatalf("repeat completed cancellation: %v", err)
			}
			assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelled, "GracefulCancellation")
		})
	}
}

func TestCancellationAfterObservedJobSuccessPreservesSuccess(t *testing.T) {
	run, job, pod := cancellationObjects(t, "cancel-after-success", true)
	completed := metav1.NewTime(fixedReconcileTime.Add(-time.Minute))
	job.Status.Succeeded = 1
	job.Status.CompletionTime = &completed
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: completed}}
	pod.Status.Phase = corev1.PodSucceeded
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
		ExitCode: 0, FinishedAt: completed, Message: `{"schemaVersion":1,"artifactManifestRef":"s3://artifacts/result.json"}`,
	}}
	kubernetesClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(job, pod).Build()
	reconciler := &AgentRunReconciler{Client: kubernetesClient, Now: func() time.Time { return fixedReconcileTime }}

	if _, err := reconciler.reconcileCancellation(context.Background(), run); err != nil {
		t.Fatalf("reconcile completed Job: %v", err)
	}
	if run.Status.Phase != executionv1alpha1.AgentRunPhaseSucceeded || run.Status.ArtifactManifestRef == "" {
		t.Fatalf("completion did not win cancellation race: %#v", run.Status)
	}
	if err := kubernetesClient.Get(context.Background(), client.ObjectKeyFromObject(job), &batchv1.Job{}); err != nil {
		t.Fatalf("successful Job was deleted: %v", err)
	}
}

func TestCancellationDeadlineSurvivesControllerRestartAndForcesTermination(t *testing.T) {
	run, job, pod := cancellationObjects(t, "cancel-restart", true)
	spoofedPod := pod.DeepCopy()
	spoofedPod.Name = "cancel-restart-spoofed"
	spoofedPod.OwnerReferences[0].UID = "different-job-uid"
	baseClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(job, pod, spoofedPod).Build()
	holdingClient := &holdGracefulDeleteClient{Client: baseClient}
	first := &AgentRunReconciler{Client: holdingClient, Now: func() time.Time { return fixedReconcileTime }}

	if _, err := first.reconcileCancellation(context.Background(), run); err != nil {
		t.Fatalf("begin cancellation: %v", err)
	}
	assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelling, "GracefulTerminationPending")
	if holdingClient.gracefulJobDeletes != 1 {
		t.Fatalf("graceful foreground Job deletes=%d, want 1", holdingClient.gracefulJobDeletes)
	}
	meta.FindStatusCondition(run.Status.Conditions, ConditionCancellationComplete).Reason = "GracefulCancellationFailed"

	restarted := &AgentRunReconciler{Client: holdingClient, Now: func() time.Time { return fixedReconcileTime.Add(cancellationDeadline + time.Second) }}
	if _, err := restarted.reconcileCancellation(context.Background(), run); err != nil {
		t.Fatalf("force cancellation after restart: %v", err)
	}
	assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelling, "ForcedTerminationPending")
	if holdingClient.forcedPodDeletes != 1 || holdingClient.forcedJobDeletes != 1 {
		t.Fatalf("forced deletes: pods=%d jobs=%d", holdingClient.forcedPodDeletes, holdingClient.forcedJobDeletes)
	}
	if err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(spoofedPod), &corev1.Pod{}); err != nil {
		t.Fatalf("label-matching Pod with a different owner UID was deleted: %v", err)
	}
	if _, err := restarted.reconcileCancellation(context.Background(), run); err != nil {
		t.Fatalf("complete forced cancellation: %v", err)
	}
	assertCancellation(t, run, executionv1alpha1.AgentRunPhaseCancelled, "ForcedCancellation")
}

func cancellationTestRun(t *testing.T, name string) *executionv1alpha1.AgentRun {
	t.Helper()
	run := validControllerAgentRun(name)
	run.UID = types.UID("run-" + name)
	run.Spec.DesiredState = executionv1alpha1.DesiredStateCancelled
	run.Status.Attempt = run.Spec.Attempt
	run.Status.Namespace = run.Namespace
	return run
}

func cancellationObjects(t *testing.T, name string, withPod bool) (*executionv1alpha1.AgentRun, *batchv1.Job, *corev1.Pod) {
	t.Helper()
	run := cancellationTestRun(t, name)
	names, err := naming.ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive resource names: %v", err)
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: names.Job, Namespace: run.Namespace, UID: types.UID("job-" + name),
		OwnerReferences: []metav1.OwnerReference{{APIVersion: executionv1alpha1.GroupVersion.String(), Kind: "AgentRun", Name: run.Name, UID: run.UID, Controller: boolPointer(true)}},
	}}
	run.Status.JobName = job.Name
	run.Status.Phase = executionv1alpha1.AgentRunPhaseProvisioning
	if !withPod {
		return run, job, nil
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: name + "-pod", Namespace: run.Namespace,
		Labels:          map[string]string{batchv1.JobNameLabel: job.Name},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job", Name: job.Name, UID: job.UID, Controller: boolPointer(true)}},
	}, Status: corev1.PodStatus{
		Phase:             corev1.PodRunning,
		ContainerStatuses: []corev1.ContainerStatus{{Name: "runner", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(fixedReconcileTime.Add(-time.Minute))}}}},
	}}
	run.Status.PodName = pod.Name
	run.Status.Phase = executionv1alpha1.AgentRunPhaseRunning
	return run, job, pod
}

func assertCancellation(t *testing.T, run *executionv1alpha1.AgentRun, phase executionv1alpha1.AgentRunPhase, reason string) {
	t.Helper()
	condition := meta.FindStatusCondition(run.Status.Conditions, ConditionCancellationComplete)
	if run.Status.Phase != phase || condition == nil || condition.Reason != reason {
		t.Fatalf("unexpected cancellation status: phase=%s condition=%#v", run.Status.Phase, condition)
	}
	if len(run.Status.Attempts) != 1 || run.Status.Attempts[0].Phase != phase {
		t.Fatalf("attempt metadata did not follow cancellation: %#v", run.Status.Attempts)
	}
}

type holdGracefulDeleteClient struct {
	client.Client
	gracefulJobDeletes int
	forcedPodDeletes   int
	forcedJobDeletes   int
}

func (c *holdGracefulDeleteClient) Delete(ctx context.Context, object client.Object, options ...client.DeleteOption) error {
	deleteOptions := (&client.DeleteOptions{}).ApplyOptions(options)
	if deleteOptions.GracePeriodSeconds == nil || *deleteOptions.GracePeriodSeconds != 0 {
		if _, isJob := object.(*batchv1.Job); isJob && deleteOptions.PropagationPolicy != nil &&
			*deleteOptions.PropagationPolicy == metav1.DeletePropagationForeground {
			c.gracefulJobDeletes++
		}
		return nil
	}
	switch object.(type) {
	case *corev1.Pod:
		c.forcedPodDeletes++
	case *batchv1.Job:
		c.forcedJobDeletes++
	}
	err := c.Client.Delete(ctx, object, options...)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
