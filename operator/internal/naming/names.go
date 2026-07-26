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
	"fmt"
	"strings"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

// ResourceNames are deterministic, DNS-label-safe names for one attempt.
type ResourceNames struct {
	ServiceAccount string
	Configuration  string
	Workspace      string
	NetworkPolicy  string
	Job            string
}

// ForAgentRun derives every child name from immutable run identity and the observed attempt.
func ForAgentRun(run *executionv1alpha1.AgentRun) (ResourceNames, error) {
	if !IsUUIDv7(string(run.Spec.RunID)) {
		return ResourceNames{}, fmt.Errorf("invalid immutable run identity")
	}
	attempt := CurrentAttempt(run)
	if attempt < 1 || attempt > run.Spec.RetryPolicy.MaxAttempts || attempt > 10 {
		return ResourceNames{}, fmt.Errorf("invalid current attempt")
	}
	runID := strings.ReplaceAll(string(run.Spec.RunID), "-", "")
	base := fmt.Sprintf("ar-%s-a%d", runID, attempt)
	return ResourceNames{
		ServiceAccount: base + "-sa",
		Configuration:  base + "-config",
		Workspace:      base + "-workspace",
		NetworkPolicy:  base + "-network",
		Job:            base + "-job",
	}, nil
}

// CurrentAttempt returns the controller-observed attempt or the scheduled attempt before status initialization.
func CurrentAttempt(run *executionv1alpha1.AgentRun) int32 {
	if run.Status.Attempt >= run.Spec.Attempt {
		return run.Status.Attempt
	}
	return run.Spec.Attempt
}

// IsUUIDv7 reports whether value is a lowercase RFC 9562 UUID version 7.
func IsUUIDv7(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' {
		return false
	}
	if !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	return isLowerHex(strings.ReplaceAll(value, "-", ""))
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
