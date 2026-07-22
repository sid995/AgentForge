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

package naming

import (
	"testing"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

func TestForAgentRunUsesObservedAttempt(t *testing.T) {
	run := &executionv1alpha1.AgentRun{
		Spec: executionv1alpha1.AgentRunSpec{
			RunID: "019b0000-0000-7000-8000-000000000004", Attempt: 1,
			RetryPolicy: executionv1alpha1.RetryPolicy{MaxAttempts: 3},
		},
		Status: executionv1alpha1.AgentRunStatus{Attempt: 2},
	}
	names, err := ForAgentRun(run)
	if err != nil {
		t.Fatalf("derive names: %v", err)
	}
	if names.Job != "ar-019b0000000070008000000000000004-a2-job" || CurrentAttempt(run) != 2 {
		t.Fatalf("unexpected attempt names: %#v", names)
	}
}

func TestForAgentRunRejectsInvalidIdentityAndAttempt(t *testing.T) {
	for _, run := range []*executionv1alpha1.AgentRun{
		{Spec: executionv1alpha1.AgentRunSpec{RunID: "invalid", Attempt: 1, RetryPolicy: executionv1alpha1.RetryPolicy{MaxAttempts: 1}}},
		{Spec: executionv1alpha1.AgentRunSpec{RunID: "019b0000-0000-7000-8000-000000000004", Attempt: 2, RetryPolicy: executionv1alpha1.RetryPolicy{MaxAttempts: 1}}},
	} {
		if _, err := ForAgentRun(run); err == nil {
			t.Fatalf("invalid run was accepted: %#v", run.Spec)
		}
	}
}
