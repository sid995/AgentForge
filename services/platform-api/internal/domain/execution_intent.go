package domain

import (
	"fmt"
	"regexp"
)

var desiredRuntimePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,119}$`)
var desiredProfilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
var desiredDigestImagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)
var desiredLocalReferencePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
var desiredTaskReferencePattern = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,31}://[^[:space:]]+$`)

// Validate enforces the complete CR desired-state boundary before persistence
// and again before Kubernetes projection.
func (desired AgentRunDesiredState) Validate(attempt int) error {
	if len(desired.RunnerImage) > 512 ||
		!desiredDigestImagePattern.MatchString(desired.RunnerImage) ||
		!desiredRuntimePattern.MatchString(desired.Runtime) ||
		!desiredProfilePattern.MatchString(desired.ExecutionProfile) ||
		len(desired.TaskReference) > 2048 || !desiredTaskReferencePattern.MatchString(desired.TaskReference) ||
		attempt < 1 || attempt > 10 || desired.MaxAttempts < attempt || desired.MaxAttempts > 10 ||
		desired.TimeoutSeconds < 1 || desired.TimeoutSeconds > 86400 ||
		desired.InitialBackoffSeconds < 1 || desired.InitialBackoffSeconds > 3600 ||
		desired.MaxBackoffSeconds < desired.InitialBackoffSeconds || desired.MaxBackoffSeconds > 21600 ||
		desired.CPUMillis < 1 || desired.CPUMillis > 128000 ||
		desired.MemoryMiB < 1 || desired.MemoryMiB > 524288 ||
		desired.CPULimitMillis < desired.CPUMillis || desired.CPULimitMillis > 128000 ||
		desired.MemoryLimitMiB < desired.MemoryMiB || desired.MemoryLimitMiB > 524288 ||
		desired.WorkspaceSizeGiB < 1 || desired.WorkspaceSizeGiB > 2048 ||
		(desired.StorageClassName != "" && !validDesiredReference(desired.StorageClassName)) ||
		(desired.WorkspaceRetentionPolicy != "Delete" && desired.WorkspaceRetentionPolicy != "Retain") ||
		(desired.NetworkProfile != "Isolated" && desired.NetworkProfile != "RestrictedEgress") ||
		!validDesiredReference(desired.ArtifactDestinationRef) ||
		(desired.NetworkProfile == "Isolated" && desired.AllowedDestinationsRef != "") ||
		(desired.NetworkProfile == "RestrictedEgress" && !validDesiredReference(desired.AllowedDestinationsRef)) {
		return fmt.Errorf("AgentRun desired state is invalid")
	}
	seenCategories := make(map[FailureCategory]struct{}, len(desired.RetryableFailureCategories))
	for _, category := range desired.RetryableFailureCategories {
		if category != FailureTransientDependency && category != FailureInternal {
			return fmt.Errorf("AgentRun desired retry category is invalid")
		}
		if _, exists := seenCategories[category]; exists {
			return fmt.Errorf("AgentRun desired retry category is duplicated")
		}
		seenCategories[category] = struct{}{}
	}
	if !validDesiredReferences(desired.ConfigurationRefs) || !validDesiredReferences(desired.SecretRefs) {
		return fmt.Errorf("AgentRun desired references are invalid")
	}
	return nil
}

func validDesiredReferences(references []string) bool {
	if len(references) > 32 {
		return false
	}
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if !validDesiredReference(reference) {
			return false
		}
		if _, exists := seen[reference]; exists {
			return false
		}
		seen[reference] = struct{}{}
	}
	return true
}

func validDesiredReference(reference string) bool {
	return len(reference) <= 253 && desiredLocalReferencePattern.MatchString(reference)
}
