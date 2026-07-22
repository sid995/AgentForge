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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
	"github.com/sid995/agentforge/operator/internal/naming"
)

func retryAllowed(run *executionv1alpha1.AgentRun) bool {
	if run.Status.Phase != executionv1alpha1.AgentRunPhaseFailed || run.Status.Attempt != naming.CurrentAttempt(run) ||
		run.Status.Attempt >= run.Spec.RetryPolicy.MaxAttempts {
		return false
	}
	switch run.Status.FailureCategory {
	case executionv1alpha1.FailureCategoryTransientDependency, executionv1alpha1.FailureCategoryInternal:
		return slices.Contains(run.Spec.RetryPolicy.RetryableFailureCategories, run.Status.FailureCategory)
	default:
		return false
	}
}

func (r *AgentRunReconciler) reconcileRetry(run *executionv1alpha1.AgentRun) ctrl.Result {
	currentAttempt := naming.CurrentAttempt(run)
	nextAttempt := currentAttempt + 1
	anchor := retryAnchor(run)
	if anchor.IsZero() {
		anchor = r.now()
	}
	delay := retryDelay(run, currentAttempt, nextAttempt)
	readyAt := anchor.Add(delay)
	if remaining := readyAt.Sub(r.now()); remaining > 0 {
		r.setCondition(run, ConditionRetryReady, metav1.ConditionFalse, "BackoffPending", fmt.Sprintf("Retry attempt %d is waiting for bounded backoff", nextAttempt))
		r.setCondition(run, ConditionReady, metav1.ConditionFalse, "RetryBackoff", "Execution retry is waiting for bounded backoff")
		return ctrl.Result{RequeueAfter: remaining}
	}

	run.Status.Attempt = nextAttempt
	run.Status.Phase = executionv1alpha1.AgentRunPhasePending
	run.Status.JobName = ""
	run.Status.PodName = ""
	run.Status.StartTime = nil
	run.Status.CompletionTime = nil
	run.Status.HeartbeatTime = nil
	run.Status.FailureCategory = ""
	run.Status.FailureReason = ""
	run.Status.ArtifactManifestRef = ""
	run.Status.ObservedGeneration = run.Generation
	r.setCondition(run, ConditionRetryReady, metav1.ConditionTrue, "RetryStarted", fmt.Sprintf("Retry attempt %d is ready for reconciliation", nextAttempt))
	r.setCondition(run, ConditionJobReady, metav1.ConditionFalse, "RetryPending", "A new deterministic execution Job has not been created")
	r.setCondition(run, ConditionReady, metav1.ConditionFalse, "RetryPending", "Execution retry resources have not been reconciled")
	r.upsertAttemptStatus(run)
	return ctrl.Result{RequeueAfter: time.Millisecond}
}

func retryAnchor(run *executionv1alpha1.AgentRun) time.Time {
	if run.Status.CompletionTime != nil && !run.Status.CompletionTime.IsZero() {
		return run.Status.CompletionTime.Time
	}
	for index := range run.Status.Conditions {
		condition := &run.Status.Conditions[index]
		if condition.Type == ConditionRetryReady && condition.Status == metav1.ConditionFalse {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

func retryDelay(run *executionv1alpha1.AgentRun, currentAttempt, nextAttempt int32) time.Duration {
	base := time.Duration(run.Spec.RetryPolicy.InitialBackoffSeconds) * time.Second
	maximum := time.Duration(run.Spec.RetryPolicy.MaxBackoffSeconds) * time.Second
	exponent := currentAttempt - run.Spec.Attempt
	for exponent > 0 && base < maximum {
		if base > maximum/2 {
			base = maximum
			break
		}
		base *= 2
		exponent--
	}
	if base > maximum {
		base = maximum
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", run.Spec.RunID, nextAttempt)))
	// Stable 50-100% jitter avoids synchronized retries and remains identical
	// across controller restarts without persisting random state.
	permille := int64(500 + binary.BigEndian.Uint64(hash[:8])%501)
	return time.Duration(int64(base) * permille / 1000)
}
