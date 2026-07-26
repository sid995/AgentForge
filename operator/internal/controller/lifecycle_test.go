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
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObserveLifecycleStates(t *testing.T) {
	started := metav1.NewTime(time.Date(2026, time.July, 22, 13, 0, 0, 0, time.UTC))
	finished := metav1.NewTime(started.Add(2 * time.Minute))
	tests := []struct {
		name     string
		mutate   func(*batchv1.Job, *corev1.Pod)
		withPod  bool
		phase    string
		reason   string
		category string
		artifact string
	}{
		{name: "Pod pending", withPod: true, phase: "Provisioning", reason: "PodPending"},
		{name: "Job active", phase: "Provisioning", reason: "JobActive", mutate: func(job *batchv1.Job, _ *corev1.Pod) { job.Status.Active = 1 }},
		{name: "Pod scheduled", withPod: true, phase: "Provisioning", reason: "PodScheduled", mutate: func(_ *batchv1.Job, pod *corev1.Pod) {
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}}
		}},
		{name: "Pod unschedulable", withPod: true, phase: "Provisioning", reason: "PodUnschedulable", mutate: func(_ *batchv1.Job, pod *corev1.Pod) {
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable}}
		}},
		{name: "container creating", withPod: true, phase: "Provisioning", reason: "ContainerCreating", mutate: waitingMutation("ContainerCreating")},
		{name: "Job and runner active", withPod: true, phase: "Running", reason: "RunnerActive", mutate: func(job *batchv1.Job, pod *corev1.Pod) {
			job.Status.Active = 1
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "runner", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: started}}}}
		}},
		{name: "Pod running before status", withPod: true, phase: "Running", reason: "PodRunning", mutate: func(_ *batchv1.Job, pod *corev1.Pod) { pod.Status.Phase = corev1.PodRunning }},
		{name: "successful process waiting for Job", withPod: true, phase: "Provisioning", reason: "JobCompletionPending", mutate: terminatedMutation(0, "Completed", "", finished)},
		{name: "Job succeeded with evidence", withPod: true, phase: "Succeeded", reason: "ResultEvidenceReady", artifact: "s3://agentforge/results/manifest.json", mutate: func(job *batchv1.Job, pod *corev1.Pod) {
			job.Status.Succeeded = 1
			job.Status.CompletionTime = &finished
			terminatedMutation(0, "Completed", `{"schemaVersion":1,"artifactManifestRef":"s3://agentforge/results/manifest.json"}`, finished)(job, pod)
		}},
		{name: "Job success without evidence", withPod: true, phase: "Failed", reason: "ResultEvidenceInvalid", category: "EXECUTION", mutate: func(job *batchv1.Job, pod *corev1.Pod) {
			job.Status.Succeeded = 1
			terminatedMutation(0, "Completed", "", finished)(job, pod)
		}},
		{name: "completed Job waiting for Pod", phase: "Provisioning", reason: "ResultEvidencePending", mutate: func(job *batchv1.Job, _ *corev1.Pod) { job.Status.Succeeded = 1 }},
		{name: "deadline exceeded", phase: "Failed", reason: "DeadlineExceeded", category: "EXECUTION", mutate: failedJobMutation("DeadlineExceeded", finished)},
		{name: "generic Job failure", phase: "Failed", reason: "JobFailed", category: "EXECUTION", mutate: failedJobMutation("BackoffLimitExceeded", finished)},
		{name: "failed count without condition", phase: "Failed", reason: "JobFailed", category: "EXECUTION", mutate: func(job *batchv1.Job, _ *corev1.Pod) { job.Status.Failed = 1 }},
		{name: "failure target", phase: "Failed", reason: "DeadlineExceeded", category: "EXECUTION", mutate: func(job *batchv1.Job, _ *corev1.Pod) {
			job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded", LastTransitionTime: finished}}
		}},
		{name: "OOMKilled", withPod: true, phase: "Failed", reason: "OOMKilled", category: "EXECUTION", mutate: terminatedMutation(137, "OOMKilled", "", finished)},
		{name: "process nonzero exit", withPod: true, phase: "Failed", reason: "ProcessExitNonZero", category: "EXECUTION", mutate: terminatedMutation(2, "Error", "", finished)},
		{name: "eviction", withPod: true, phase: "Failed", reason: "PodEvicted", category: "TRANSIENT_DEPENDENCY", mutate: func(_ *batchv1.Job, pod *corev1.Pod) { pod.Status.Reason = "Evicted" }},
		{name: "node loss", withPod: true, phase: "Failed", reason: "NodeLost", category: "TRANSIENT_DEPENDENCY", mutate: func(_ *batchv1.Job, pod *corev1.Pod) { pod.Status.Reason = "NodeLost" }},
		{name: "image pull failure", withPod: true, phase: "Failed", reason: "ImagePullFailed", category: "TRANSIENT_DEPENDENCY", mutate: waitingMutation("ImagePullBackOff")},
		{name: "invalid image", withPod: true, phase: "Failed", reason: "InvalidImage", category: "PERMANENT_DEPENDENCY", mutate: waitingMutation("InvalidImageName")},
		{name: "configuration error", withPod: true, phase: "Failed", reason: "ConfigurationError", category: "POLICY", mutate: waitingMutation("CreateContainerConfigError")},
		{name: "container start failure", withPod: true, phase: "Failed", reason: "ContainerStartFailed", category: "PERMANENT_DEPENDENCY", mutate: waitingMutation("CreateContainerError")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"}, Status: batchv1.JobStatus{StartTime: &started}}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod", OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job"))},
				},
				Status: corev1.PodStatus{StartTime: &started},
			}
			if test.mutate != nil {
				test.mutate(job, pod)
			}
			pods := []corev1.Pod(nil)
			if test.withPod {
				pods = []corev1.Pod{*pod}
			}
			observation := observeLifecycle(job, pods)
			if string(observation.phase) != test.phase || observation.reason != test.reason ||
				string(observation.failureCategory) != test.category || observation.artifactManifestRef != test.artifact {
				t.Fatalf("unexpected observation: %#v", observation)
			}
			if test.withPod && observation.podName != pod.Name {
				t.Fatalf("Pod name was not projected: %#v", observation)
			}
			if observation.phase == "Failed" && observation.failureReason == "" {
				t.Fatal("failed observation has no bounded diagnostic")
			}
		})
	}
}

func TestLifecycleIgnoresSpoofedPodLabels(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "trusted-job"}}
	spoofed := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "spoofed"},
		Status:     corev1.PodStatus{Reason: "Evicted"},
	}
	observation := observeLifecycle(job, []corev1.Pod{spoofed})
	if observation.phase != "Provisioning" || observation.reason != "PodPending" || observation.podName != "" {
		t.Fatalf("unowned Pod influenced lifecycle: %#v", observation)
	}
}

func TestTerminationEvidenceContract(t *testing.T) {
	valid := `{"schemaVersion":1,"artifactManifestRef":"artifact+https://store.example/manifests/run.json"}`
	evidence, err := parseTerminationEvidence(valid)
	if err != nil || evidence.ArtifactManifestRef == "" {
		t.Fatalf("valid evidence: evidence=%#v error=%v", evidence, err)
	}
	invalid := []string{
		"", "not-json", `{"schemaVersion":2,"artifactManifestRef":"s3://bucket/result"}`,
		`{"schemaVersion":1,"artifactManifestRef":"relative"}`,
		`{"schemaVersion":1,"artifactManifestRef":"s3://bucket/result","secret":"leak"}`,
		valid + " trailing", strings.Repeat("x", maximumTerminationEvidenceBytes+1),
	}
	for _, value := range invalid {
		if _, err := parseTerminationEvidence(value); err == nil {
			t.Fatalf("invalid evidence was accepted: %.80q", value)
		}
	}
}

func TestLifecycleProjectionMaintainsAttemptMetadata(t *testing.T) {
	run := validControllerAgentRun("attempt-projection")
	run.Status.Attempt = 1
	run.Status.JobName = "attempt-one-job"
	reconciler := &AgentRunReconciler{Now: func() time.Time { return fixedReconcileTime }}
	completion := metav1.NewTime(fixedReconcileTime)
	reconciler.projectLifecycle(run, lifecycleObservation{
		phase: "Failed", reason: "OOMKilled", message: "Runner exceeded its memory limit",
		podName: "attempt-one-pod", completionTime: &completion,
		failureCategory: "EXECUTION", failureReason: "Runner exceeded its memory limit",
	})
	if len(run.Status.Attempts) != 1 || run.Status.Attempts[0].PodName != "attempt-one-pod" || !currentAttemptTerminal(run) {
		t.Fatalf("attempt metadata was not projected: %#v", run.Status)
	}
	reconciler.projectLifecycle(run, lifecycleObservation{
		phase: "Succeeded", reason: "ResultEvidenceReady", message: "Execution completed with mandatory result evidence",
		podName: "attempt-one-pod", completionTime: &completion, artifactManifestRef: "s3://bucket/result",
	})
	if len(run.Status.Attempts) != 1 || run.Status.Attempts[0].Phase != "Succeeded" || run.Status.Attempts[0].FailureCategory != "" {
		t.Fatalf("attempt metadata was not updated in place: %#v", run.Status.Attempts)
	}
}

func waitingMutation(reason string) func(*batchv1.Job, *corev1.Pod) {
	return func(_ *batchv1.Job, pod *corev1.Pod) {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "runner", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}}
	}
}

func terminatedMutation(exitCode int32, reason, message string, finished metav1.Time) func(*batchv1.Job, *corev1.Pod) {
	return func(_ *batchv1.Job, pod *corev1.Pod) {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "runner", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: exitCode, Reason: reason, Message: message, FinishedAt: finished,
		}}}}
	}
}

func failedJobMutation(reason string, finished metav1.Time) func(*batchv1.Job, *corev1.Pod) {
	return func(job *batchv1.Job, _ *corev1.Pod) {
		job.Status.Failed = 1
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: reason, LastTransitionTime: finished,
		}}
	}
}
