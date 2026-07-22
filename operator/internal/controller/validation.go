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
	"fmt"
	"net/url"
	"strings"

	executionv1alpha1 "github.com/sid995/agentforge/operator/api/v1alpha1"
)

func validateAgentRun(run *executionv1alpha1.AgentRun) error {
	if run.Generation < 1 {
		return fmt.Errorf("metadata generation must be positive")
	}
	identifiers := []struct {
		name  string
		value executionv1alpha1.UUIDv7
	}{
		{name: "tenantId", value: run.Spec.TenantID},
		{name: "projectId", value: run.Spec.ProjectID},
		{name: "runId", value: run.Spec.RunID},
		{name: "attemptId", value: run.Spec.AttemptID},
	}
	for _, identifier := range identifiers {
		if !isUUIDv7(string(identifier.value)) {
			return fmt.Errorf("%s must be a lowercase UUIDv7", identifier.name)
		}
	}
	if run.Spec.Attempt < 1 || run.Spec.Attempt > 10 || run.Spec.Attempt > run.Spec.RetryPolicy.MaxAttempts {
		return fmt.Errorf("attempt must be within the retry ceiling")
	}
	if !isDigestImage(run.Spec.RunnerImage) {
		return fmt.Errorf("runnerImage must use a sha256 digest")
	}
	if !isSecureReference(run.Spec.TaskRef) {
		return fmt.Errorf("taskRef must be a secure reference")
	}
	if run.Spec.TimeoutSeconds < 1 || run.Spec.TimeoutSeconds > 86400 {
		return fmt.Errorf("timeoutSeconds is outside the approved range")
	}
	if run.Spec.Resources.Limits.CPUMillis < run.Spec.Resources.Requests.CPUMillis ||
		run.Spec.Resources.Limits.MemoryMiB < run.Spec.Resources.Requests.MemoryMiB {
		return fmt.Errorf("resource limits must cover requests")
	}
	if err := validateNetwork(run.Spec.Network); err != nil {
		return err
	}
	if run.Spec.DesiredState != executionv1alpha1.DesiredStateRunning &&
		run.Spec.DesiredState != executionv1alpha1.DesiredStateCancelled {
		return fmt.Errorf("desiredState is not supported")
	}
	if _, err := NamesForAgentRun(run); err != nil {
		return err
	}
	return nil
}

func isUUIDv7(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' {
		return false
	}
	if !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	return isLowerHex(strings.ReplaceAll(value, "-", ""))
}

func isDigestImage(value string) bool {
	separator := strings.LastIndex(value, "@sha256:")
	if separator < 1 || separator+8+64 != len(value) || strings.ContainsAny(value[:separator], " \t\r\n") {
		return false
	}
	return isLowerHex(value[separator+8:])
}

func isSecureReference(value string) bool {
	if len(value) > 2048 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && len(parsed.Scheme) >= 2 && parsed.Scheme == strings.ToLower(parsed.Scheme) && (parsed.Host != "" || parsed.Opaque != "")
}

func validateNetwork(network executionv1alpha1.NetworkSpec) error {
	switch network.Profile {
	case executionv1alpha1.NetworkProfileIsolated:
		if network.AllowedDestinationsRef != nil {
			return fmt.Errorf("isolated network cannot include allowed destinations")
		}
	case executionv1alpha1.NetworkProfileRestrictedEgress:
		if network.AllowedDestinationsRef == nil {
			return fmt.Errorf("restricted egress requires an allowed destination reference")
		}
	default:
		return fmt.Errorf("network profile is not supported")
	}
	return nil
}
