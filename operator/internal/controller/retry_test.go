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
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

func TestRetryPolicyAllowsOnlyApprovedAllowlistedCategories(t *testing.T) {
	for _, category := range []executionv1alpha1.FailureCategory{
		executionv1alpha1.FailureCategoryTransientDependency,
		executionv1alpha1.FailureCategoryInternal,
	} {
		run := retryTestRun(t, "retry-allowed-"+string(category), category)
		if !retryAllowed(run) {
			t.Errorf("approved allowlisted category %s was rejected", category)
		}
	}

	for _, category := range []executionv1alpha1.FailureCategory{
		executionv1alpha1.FailureCategoryValidation,
		executionv1alpha1.FailureCategoryAuthentication,
		executionv1alpha1.FailureCategoryAuthorization,
		executionv1alpha1.FailureCategoryQuota,
		executionv1alpha1.FailureCategoryConflict,
		executionv1alpha1.FailureCategoryPermanentDependency,
		executionv1alpha1.FailureCategoryExecution,
		executionv1alpha1.FailureCategoryPolicy,
	} {
		run := retryTestRun(t, "retry-denied-"+string(category), category)
		run.Spec.RetryPolicy.RetryableFailureCategories = append(run.Spec.RetryPolicy.RetryableFailureCategories, category)
		if retryAllowed(run) {
			t.Errorf("unapproved category %s was accepted", category)
		}
	}
}

func TestRetryRequiresAllowlistAndRemainingAttempt(t *testing.T) {
	run := retryTestRun(t, "retry-policy-boundaries", executionv1alpha1.FailureCategoryTransientDependency)
	run.Spec.RetryPolicy.RetryableFailureCategories = nil
	if retryAllowed(run) {
		t.Fatal("empty desired allowlist enabled a retry")
	}
	run.Spec.RetryPolicy.RetryableFailureCategories = []executionv1alpha1.FailureCategory{executionv1alpha1.FailureCategoryTransientDependency}
	run.Status.Attempt = run.Spec.RetryPolicy.MaxAttempts
	if retryAllowed(run) {
		t.Fatal("maximum attempt was retried")
	}
	run.Status.Attempt = 1
	run.Status.Phase = executionv1alpha1.AgentRunPhaseSucceeded
	if retryAllowed(run) {
		t.Fatal("successful execution was retried")
	}
	run.Status.Phase = executionv1alpha1.AgentRunPhaseCancelled
	if retryAllowed(run) {
		t.Fatal("cancelled execution was retried")
	}
}

func TestRetryBackoffIsExponentialCappedJitteredAndRestartStable(t *testing.T) {
	run := retryTestRun(t, "retry-backoff", executionv1alpha1.FailureCategoryTransientDependency)
	run.Spec.RetryPolicy.InitialBackoffSeconds = 10
	run.Spec.RetryPolicy.MaxBackoffSeconds = 25
	run.Spec.RetryPolicy.MaxAttempts = 5

	for _, testCase := range []struct {
		current int32
		minimum time.Duration
		maximum time.Duration
	}{
		{current: 1, minimum: 5 * time.Second, maximum: 10 * time.Second},
		{current: 2, minimum: 10 * time.Second, maximum: 20 * time.Second},
		{current: 3, minimum: 12500 * time.Millisecond, maximum: 25 * time.Second},
		{current: 4, minimum: 12500 * time.Millisecond, maximum: 25 * time.Second},
	} {
		delay := retryDelay(run, testCase.current, testCase.current+1)
		if delay < testCase.minimum || delay > testCase.maximum {
			t.Errorf("attempt %d delay %s outside [%s,%s]", testCase.current, delay, testCase.minimum, testCase.maximum)
		}
		if restarted := retryDelay(run.DeepCopy(), testCase.current, testCase.current+1); restarted != delay {
			t.Errorf("attempt %d delay changed after restart: %s != %s", testCase.current, restarted, delay)
		}
	}
}

func TestRetryBackoffAndAttemptAdvanceSurviveRestart(t *testing.T) {
	run := retryTestRun(t, "retry-restart", executionv1alpha1.FailureCategoryInternal)
	failedCompletion := metav1.NewTime(fixedReconcileTime)
	run.Status.CompletionTime = &failedCompletion
	first := &AgentRunReconciler{Now: func() time.Time { return fixedReconcileTime }}

	result := first.reconcileRetry(run)
	if result.RequeueAfter <= 0 || run.Status.Attempt != 1 || run.Status.Phase != executionv1alpha1.AgentRunPhaseFailed {
		t.Fatalf("unexpected initial retry backoff: result=%#v status=%#v", result, run.Status)
	}
	retryCondition := meta.FindStatusCondition(run.Status.Conditions, ConditionRetryReady)
	if retryCondition == nil || retryCondition.Reason != "BackoffPending" {
		t.Fatalf("retry backoff condition: %#v", retryCondition)
	}

	oldNames, err := naming.ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive old names: %v", err)
	}
	advanceAt := failedCompletion.Add(retryDelay(run, 1, 2) + time.Millisecond)
	restarted := &AgentRunReconciler{Now: func() time.Time { return advanceAt }}
	result = restarted.reconcileRetry(run)
	if result.RequeueAfter <= 0 || run.Status.Attempt != 2 || run.Status.Phase != executionv1alpha1.AgentRunPhasePending {
		t.Fatalf("retry did not advance after restart: result=%#v status=%#v", result, run.Status)
	}
	newNames, err := naming.ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive retry names: %v", err)
	}
	if newNames.Job == oldNames.Job || newNames.Job != "ar-"+compactRunID(run)+"-a2-job" {
		t.Fatalf("retry reused Job identity: old=%s new=%s", oldNames.Job, newNames.Job)
	}
	if run.Status.JobName != "" || run.Status.PodName != "" || run.Status.FailureCategory != "" || run.Status.CompletionTime != nil {
		t.Fatalf("current retry projection retained transient fields: %#v", run.Status)
	}
	if len(run.Status.Attempts) != 2 || run.Status.Attempts[0].Phase != executionv1alpha1.AgentRunPhaseFailed ||
		run.Status.Attempts[1].Attempt != 2 || run.Status.Attempts[1].Phase != executionv1alpha1.AgentRunPhasePending {
		t.Fatalf("prior attempt was not retained: %#v", run.Status.Attempts)
	}
}

func TestRetryWithoutCompletionTimePersistsItsOwnBackoffAnchor(t *testing.T) {
	run := retryTestRun(t, "retry-no-completion", executionv1alpha1.FailureCategoryInternal)
	delay := retryDelay(run, 1, 2)
	first := &AgentRunReconciler{Now: func() time.Time { return fixedReconcileTime }}
	firstResult := first.reconcileRetry(run)
	condition := meta.FindStatusCondition(run.Status.Conditions, ConditionRetryReady)
	if condition == nil || !condition.LastTransitionTime.Equal(&metav1.Time{Time: fixedReconcileTime}) {
		t.Fatalf("retry did not persist a backoff anchor: %#v", condition)
	}

	halfway := &AgentRunReconciler{Now: func() time.Time { return fixedReconcileTime.Add(delay / 2) }}
	halfwayResult := halfway.reconcileRetry(run)
	if halfwayResult.RequeueAfter <= 0 || halfwayResult.RequeueAfter >= firstResult.RequeueAfter {
		t.Fatalf("restart reset retry backoff: first=%s halfway=%s", firstResult.RequeueAfter, halfwayResult.RequeueAfter)
	}

	ready := &AgentRunReconciler{Now: func() time.Time { return fixedReconcileTime.Add(delay + time.Millisecond) }}
	if result := ready.reconcileRetry(run); result.RequeueAfter <= 0 || run.Status.Attempt != 2 {
		t.Fatalf("retry did not advance from persisted anchor: result=%#v status=%#v", result, run.Status)
	}
}

func retryTestRun(t *testing.T, name string, category executionv1alpha1.FailureCategory) *executionv1alpha1.AgentRun {
	t.Helper()
	run := validControllerAgentRun(name)
	run.Spec.RetryPolicy.MaxAttempts = 3
	run.Spec.RetryPolicy.RetryableFailureCategories = []executionv1alpha1.FailureCategory{
		executionv1alpha1.FailureCategoryTransientDependency,
		executionv1alpha1.FailureCategoryInternal,
	}
	run.Status.Attempt = 1
	run.Status.Phase = executionv1alpha1.AgentRunPhaseFailed
	run.Status.JobName = "attempt-one-job"
	run.Status.PodName = "attempt-one-pod"
	run.Status.FailureCategory = category
	run.Status.FailureReason = "bounded test failure"
	run.Status.Attempts = []executionv1alpha1.AttemptStatus{{
		Attempt: 1, JobName: run.Status.JobName, PodName: run.Status.PodName,
		Phase: executionv1alpha1.AgentRunPhaseFailed, FailureCategory: category, FailureReason: run.Status.FailureReason,
	}}
	return run
}

func compactRunID(run *executionv1alpha1.AgentRun) string {
	value := string(run.Spec.RunID)
	return string(slices.DeleteFunc([]byte(value), func(character byte) bool { return character == '-' }))
}
