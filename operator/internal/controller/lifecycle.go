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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

const maximumTerminationEvidenceBytes = 4096

var artifactManifestReferencePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,31}://[^\s]+$`)

type lifecycleObservation struct {
	phase               executionv1alpha1.AgentRunPhase
	reason              string
	message             string
	podName             string
	startTime           *metav1.Time
	completionTime      *metav1.Time
	failureCategory     executionv1alpha1.FailureCategory
	failureReason       string
	artifactManifestRef string
}

type terminationEvidence struct {
	SchemaVersion       int    `json:"schemaVersion"`
	ArtifactManifestRef string `json:"artifactManifestRef"`
}

func (r *AgentRunReconciler) observeJob(ctx context.Context, run *executionv1alpha1.AgentRun, job *batchv1.Job) (ctrl.Result, error) {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(job.Namespace), client.MatchingLabels{batchv1.JobNameLabel: job.Name}); err != nil {
		return ctrl.Result{}, r.handlePrerequisiteError(run, ConditionJobReady, "PodListFailed", err)
	}
	observation := observeLifecycle(job, pods.Items)
	r.projectLifecycle(run, observation)
	if observation.phase == executionv1alpha1.AgentRunPhaseProvisioning || observation.phase == executionv1alpha1.AgentRunPhaseRunning {
		return ctrl.Result{RequeueAfter: prerequisiteRequeue}, nil
	}
	return ctrl.Result{}, nil
}

func observeLifecycle(job *batchv1.Job, pods []corev1.Pod) lifecycleObservation {
	observation := lifecycleObservation{
		phase:   executionv1alpha1.AgentRunPhaseProvisioning,
		reason:  "PodPending",
		message: "Execution is waiting for an observed Pod",
	}
	if job.Status.StartTime != nil {
		observation.startTime = job.Status.StartTime.DeepCopy()
	}
	pod := selectLifecyclePod(job, pods)
	if pod != nil {
		observation.podName = pod.Name
		if observation.startTime == nil && pod.Status.StartTime != nil {
			observation.startTime = pod.Status.StartTime.DeepCopy()
		}
		if failed := terminalPodObservation(pod); failed != nil {
			if failed.completionTime == nil {
				failed.completionTime = jobCompletionTime(job)
			}
			if failed.startTime == nil {
				failed.startTime = observation.startTime
			}
			failed.podName = pod.Name
			return *failed
		}
	}

	condition := findJobCondition(job, batchv1.JobFailed)
	if condition == nil {
		condition = findJobCondition(job, batchv1.JobFailureTarget)
	}
	if condition != nil {
		category, reason, message := classifyJobFailure(condition.Reason)
		return lifecycleObservation{
			phase: executionv1alpha1.AgentRunPhaseFailed, reason: reason, message: message,
			podName: observation.podName, startTime: observation.startTime,
			completionTime:  firstTime(job.Status.CompletionTime, &condition.LastTransitionTime),
			failureCategory: category, failureReason: message,
		}
	}
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		return lifecycleObservation{
			phase: executionv1alpha1.AgentRunPhaseFailed, reason: "JobFailed",
			message: "Execution Job reported failure", podName: observation.podName,
			startTime: observation.startTime, completionTime: jobCompletionTime(job),
			failureCategory: executionv1alpha1.FailureCategoryExecution,
			failureReason:   "Execution Job reported failure",
		}
	}

	if findJobCondition(job, batchv1.JobComplete) != nil || job.Status.Succeeded > 0 {
		if pod == nil {
			observation.reason = "ResultEvidencePending"
			observation.message = "Completed Job is waiting for Pod result evidence"
			return observation
		}
		terminated := runnerTermination(pod)
		if terminated == nil {
			observation.reason = "ResultEvidencePending"
			observation.message = "Completed Job is waiting for runner termination evidence"
			return observation
		}
		evidence, err := parseTerminationEvidence(terminated.Message)
		if err != nil {
			return lifecycleObservation{
				phase: executionv1alpha1.AgentRunPhaseFailed, reason: "ResultEvidenceInvalid",
				message: "Runner completed without valid mandatory result evidence",
				podName: pod.Name, startTime: observation.startTime,
				completionTime:  firstTime(job.Status.CompletionTime, &terminated.FinishedAt),
				failureCategory: executionv1alpha1.FailureCategoryExecution,
				failureReason:   "Mandatory result evidence is missing or invalid",
			}
		}
		return lifecycleObservation{
			phase: executionv1alpha1.AgentRunPhaseSucceeded, reason: "ResultEvidenceReady",
			message: "Execution completed with mandatory result evidence",
			podName: pod.Name, startTime: observation.startTime,
			completionTime:      firstTime(job.Status.CompletionTime, &terminated.FinishedAt),
			artifactManifestRef: evidence.ArtifactManifestRef,
		}
	}

	if pod == nil {
		if job.Status.Active > 0 {
			observation.reason = "JobActive"
			observation.message = "Execution Job is active and waiting for a Pod"
		}
		return observation
	}
	return observeNonTerminalPod(observation, pod)
}

func terminalPodObservation(pod *corev1.Pod) *lifecycleObservation {
	switch pod.Status.Reason {
	case "Evicted":
		return failedObservation("PodEvicted", "Execution Pod was evicted", executionv1alpha1.FailureCategoryTransientDependency, pod)
	case "NodeLost":
		return failedObservation("NodeLost", "Execution Pod was lost with its node", executionv1alpha1.FailureCategoryTransientDependency, pod)
	}
	container := runnerContainerStatus(pod)
	if container == nil {
		return nil
	}
	if waiting := container.State.Waiting; waiting != nil {
		switch waiting.Reason {
		case "ErrImagePull", "ImagePullBackOff":
			return failedObservation("ImagePullFailed", "Runner image could not be pulled", executionv1alpha1.FailureCategoryTransientDependency, pod)
		case "InvalidImageName":
			return failedObservation("InvalidImage", "Runner image reference was rejected", executionv1alpha1.FailureCategoryPermanentDependency, pod)
		case "CreateContainerConfigError":
			return failedObservation("ConfigurationError", "Runner container configuration was rejected", executionv1alpha1.FailureCategoryPolicy, pod)
		case "CreateContainerError", "RunContainerError":
			return failedObservation("ContainerStartFailed", "Runner container could not start", executionv1alpha1.FailureCategoryPermanentDependency, pod)
		}
	}
	terminated := container.State.Terminated
	if terminated == nil || terminated.ExitCode == 0 {
		return nil
	}
	if terminated.Reason == "OOMKilled" {
		return failedObservation("OOMKilled", "Runner exceeded its memory limit", executionv1alpha1.FailureCategoryExecution, pod)
	}
	return failedObservation("ProcessExitNonZero", "Runner process exited with a nonzero status", executionv1alpha1.FailureCategoryExecution, pod)
}

func observeNonTerminalPod(observation lifecycleObservation, pod *corev1.Pod) lifecycleObservation {
	container := runnerContainerStatus(pod)
	if container != nil {
		if running := container.State.Running; running != nil {
			observation.phase = executionv1alpha1.AgentRunPhaseRunning
			observation.reason = "RunnerActive"
			observation.message = "Runner container is active"
			if !running.StartedAt.IsZero() {
				observation.startTime = running.StartedAt.DeepCopy()
			}
			return observation
		}
		if waiting := container.State.Waiting; waiting != nil {
			if waiting.Reason == "ContainerCreating" {
				observation.reason = "ContainerCreating"
				observation.message = "Runner container is being created"
			} else {
				observation.reason = "PodPending"
				observation.message = "Runner container is waiting"
			}
			return observation
		}
		if terminated := container.State.Terminated; terminated != nil && terminated.ExitCode == 0 {
			observation.reason = "JobCompletionPending"
			observation.message = "Runner exited successfully and Job completion is pending"
			return observation
		}
	}
	if condition := findPodCondition(pod, corev1.PodScheduled); condition != nil {
		if condition.Status == corev1.ConditionFalse && condition.Reason == corev1.PodReasonUnschedulable {
			observation.reason = "PodUnschedulable"
			observation.message = "Execution Pod cannot currently be scheduled"
			return observation
		}
		if condition.Status == corev1.ConditionTrue {
			observation.reason = "PodScheduled"
			observation.message = "Execution Pod is scheduled and waiting for the runner"
			return observation
		}
	}
	if pod.Status.Phase == corev1.PodRunning {
		observation.phase = executionv1alpha1.AgentRunPhaseRunning
		observation.reason = "PodRunning"
		observation.message = "Execution Pod is running"
	}
	return observation
}

func failedObservation(reason, message string, category executionv1alpha1.FailureCategory, pod *corev1.Pod) *lifecycleObservation {
	observation := &lifecycleObservation{
		phase: executionv1alpha1.AgentRunPhaseFailed, reason: reason, message: message,
		failureCategory: category, failureReason: message,
	}
	if pod.Status.StartTime != nil {
		observation.startTime = pod.Status.StartTime.DeepCopy()
	}
	if terminated := runnerTermination(pod); terminated != nil && !terminated.FinishedAt.IsZero() {
		observation.completionTime = terminated.FinishedAt.DeepCopy()
	}
	return observation
}

func classifyJobFailure(reason string) (executionv1alpha1.FailureCategory, string, string) {
	switch reason {
	case "DeadlineExceeded":
		return executionv1alpha1.FailureCategoryExecution, "DeadlineExceeded", "Execution exceeded its active deadline"
	case "BackoffLimitExceeded":
		return executionv1alpha1.FailureCategoryExecution, "JobFailed", "Execution Job exhausted its zero-retry policy"
	default:
		return executionv1alpha1.FailureCategoryExecution, "JobFailed", "Execution Job reported failure"
	}
}

func (r *AgentRunReconciler) projectLifecycle(run *executionv1alpha1.AgentRun, observation lifecycleObservation) {
	run.Status.Phase = observation.phase
	run.Status.PodName = observation.podName
	run.Status.StartTime = copyTime(observation.startTime)
	run.Status.CompletionTime = copyTime(observation.completionTime)
	run.Status.FailureCategory = observation.failureCategory
	run.Status.FailureReason = observation.failureReason
	run.Status.ArtifactManifestRef = observation.artifactManifestRef
	conditionStatus := metav1.ConditionFalse
	if observation.phase == executionv1alpha1.AgentRunPhaseSucceeded {
		conditionStatus = metav1.ConditionTrue
	}
	r.setCondition(run, ConditionReady, conditionStatus, observation.reason, observation.message)
	r.upsertAttemptStatus(run)
}

func (r *AgentRunReconciler) recordMissingJob(run *executionv1alpha1.AgentRun, jobName string) {
	run.Status.JobName = jobName
	r.projectLifecycle(run, lifecycleObservation{
		phase: executionv1alpha1.AgentRunPhaseFailed, reason: "JobMissing",
		message:         "Previously observed execution Job is missing",
		podName:         run.Status.PodName,
		startTime:       copyTime(run.Status.StartTime),
		completionTime:  copyTime(run.Status.CompletionTime),
		failureCategory: executionv1alpha1.FailureCategoryInternal,
		failureReason:   "Previously observed execution Job was deleted",
	})
}

func (r *AgentRunReconciler) upsertAttemptStatus(run *executionv1alpha1.AgentRun) {
	attempt := naming.CurrentAttempt(run)
	current := executionv1alpha1.AttemptStatus{
		Attempt: attempt, JobName: run.Status.JobName, PodName: run.Status.PodName,
		Phase: run.Status.Phase, StartTime: copyTime(run.Status.StartTime), CompletionTime: copyTime(run.Status.CompletionTime),
		FailureCategory: run.Status.FailureCategory, FailureReason: run.Status.FailureReason,
		ArtifactManifestRef: run.Status.ArtifactManifestRef,
	}
	found := false
	for index := range run.Status.Attempts {
		if run.Status.Attempts[index].Attempt == attempt {
			run.Status.Attempts[index] = current
			found = true
			break
		}
	}
	if !found {
		run.Status.Attempts = append(run.Status.Attempts, current)
	}
	slices.SortFunc(run.Status.Attempts, func(left, right executionv1alpha1.AttemptStatus) int {
		return int(left.Attempt - right.Attempt)
	})
}

func currentAttemptTerminal(run *executionv1alpha1.AgentRun) bool {
	if run.Status.Attempt != naming.CurrentAttempt(run) {
		return false
	}
	switch run.Status.Phase {
	case executionv1alpha1.AgentRunPhaseSucceeded, executionv1alpha1.AgentRunPhaseFailed, executionv1alpha1.AgentRunPhaseCancelled:
		return true
	default:
		return false
	}
}

func selectLifecyclePod(job *batchv1.Job, pods []corev1.Pod) *corev1.Pod {
	candidates := make([]corev1.Pod, 0, len(pods))
	for index := range pods {
		owner := metav1.GetControllerOf(&pods[index])
		if owner != nil && owner.APIVersion == batchv1.SchemeGroupVersion.String() && owner.Kind == "Job" &&
			owner.Name == job.Name && owner.UID == job.UID {
			candidates = append(candidates, *pods[index].DeepCopy())
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortFunc(candidates, func(left, right corev1.Pod) int {
		if result := left.CreationTimestamp.Compare(right.CreationTimestamp.Time); result != 0 {
			return result
		}
		return strings.Compare(left.Name, right.Name)
	})
	return &candidates[len(candidates)-1]
}

func runnerContainerStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for index := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[index].Name == "runner" {
			return &pod.Status.ContainerStatuses[index]
		}
	}
	return nil
}

func runnerTermination(pod *corev1.Pod) *corev1.ContainerStateTerminated {
	container := runnerContainerStatus(pod)
	if container == nil {
		return nil
	}
	return container.State.Terminated
}

func parseTerminationEvidence(message string) (*terminationEvidence, error) {
	if len(message) == 0 || len(message) > maximumTerminationEvidenceBytes {
		return nil, fmt.Errorf("termination evidence size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(message))
	decoder.DisallowUnknownFields()
	evidence := &terminationEvidence{}
	if err := decoder.Decode(evidence); err != nil {
		return nil, fmt.Errorf("decode termination evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("termination evidence contains trailing data")
	}
	if evidence.SchemaVersion != 1 || len(evidence.ArtifactManifestRef) > 2048 ||
		!artifactManifestReferencePattern.MatchString(evidence.ArtifactManifestRef) {
		return nil, fmt.Errorf("termination evidence fields are invalid")
	}
	return evidence, nil
}

func findJobCondition(job *batchv1.Job, conditionType batchv1.JobConditionType) *batchv1.JobCondition {
	for index := range job.Status.Conditions {
		condition := &job.Status.Conditions[index]
		if condition.Type == conditionType && condition.Status == corev1.ConditionTrue {
			return condition
		}
	}
	return nil
}

func findPodCondition(pod *corev1.Pod, conditionType corev1.PodConditionType) *corev1.PodCondition {
	for index := range pod.Status.Conditions {
		if pod.Status.Conditions[index].Type == conditionType {
			return &pod.Status.Conditions[index]
		}
	}
	return nil
}

func jobCompletionTime(job *batchv1.Job) *metav1.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.DeepCopy()
	}
	if condition := findJobCondition(job, batchv1.JobFailed); condition != nil {
		return condition.LastTransitionTime.DeepCopy()
	}
	if condition := findJobCondition(job, batchv1.JobFailureTarget); condition != nil {
		return condition.LastTransitionTime.DeepCopy()
	}
	return nil
}

func firstTime(values ...*metav1.Time) *metav1.Time {
	for _, value := range values {
		if value != nil && !value.IsZero() {
			return value.DeepCopy()
		}
	}
	return nil
}

func copyTime(value *metav1.Time) *metav1.Time {
	if value == nil {
		return nil
	}
	return value.DeepCopy()
}
