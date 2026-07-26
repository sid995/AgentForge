package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

var immutableImagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$`)
var objectReferencePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
var executionRuntimePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,119}$`)

// ExecutionIntentConfig contains trusted platform defaults captured in each
// durable handoff intent. It is not tenant-controlled placement configuration.
type ExecutionIntentConfig struct {
	RunnerImages           map[string]string
	WorkspaceSizeGiB       int
	StorageClassName       string
	ArtifactDestinationRef string
	ConfigurationRefs      []string
	SecretRefs             []string
}

func loadExecutionIntent(lookup LookupEnv) (ExecutionIntentConfig, error) {
	configuration := ExecutionIntentConfig{}
	rawImages, ok := lookup("AGENTFORGE_SCHEDULER_RUNNER_IMAGES")
	if !ok || strings.TrimSpace(rawImages) == "" {
		return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_RUNNER_IMAGES is required")
	}
	if err := json.Unmarshal([]byte(rawImages), &configuration.RunnerImages); err != nil || len(configuration.RunnerImages) == 0 {
		return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_RUNNER_IMAGES must be a non-empty JSON object")
	}
	for runtime, image := range configuration.RunnerImages {
		if !executionRuntimePattern.MatchString(runtime) || len(image) > 512 || !immutableImagePattern.MatchString(image) {
			return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_RUNNER_IMAGES contains invalid runtime or digest")
		}
	}
	workspace, ok := lookup("AGENTFORGE_SCHEDULER_WORKSPACE_SIZE_GIB")
	if !ok {
		return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_WORKSPACE_SIZE_GIB is required")
	}
	size, err := strconv.Atoi(strings.TrimSpace(workspace))
	if err != nil || size < 1 || size > 2048 {
		return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_WORKSPACE_SIZE_GIB must be between 1 and 2048")
	}
	configuration.WorkspaceSizeGiB = size
	if value, exists := lookup("AGENTFORGE_SCHEDULER_STORAGE_CLASS"); exists {
		configuration.StorageClassName = strings.TrimSpace(value)
		if configuration.StorageClassName != "" && !validObjectReference(configuration.StorageClassName) {
			return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_STORAGE_CLASS must be a DNS subdomain name")
		}
	}
	artifact, ok := lookup("AGENTFORGE_SCHEDULER_ARTIFACT_DESTINATION_REF")
	configuration.ArtifactDestinationRef = strings.TrimSpace(artifact)
	if !ok || !validObjectReference(configuration.ArtifactDestinationRef) {
		return configuration, fmt.Errorf("AGENTFORGE_SCHEDULER_ARTIFACT_DESTINATION_REF must be a DNS subdomain name")
	}
	if configuration.ConfigurationRefs, err = referenceList(lookup, "AGENTFORGE_SCHEDULER_CONFIGURATION_REFS"); err != nil {
		return configuration, err
	}
	if configuration.SecretRefs, err = referenceList(lookup, "AGENTFORGE_SCHEDULER_SECRET_REFS"); err != nil {
		return configuration, err
	}
	return configuration, nil
}

func referenceList(lookup LookupEnv, name string) ([]string, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 32 {
		return nil, fmt.Errorf("%s must contain at most 32 references", name)
	}
	seen := make(map[string]struct{}, len(parts))
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
		if !validObjectReference(parts[index]) {
			return nil, fmt.Errorf("%s contains an invalid DNS subdomain name", name)
		}
		if _, exists := seen[parts[index]]; exists {
			return nil, fmt.Errorf("%s contains a duplicate reference", name)
		}
		seen[parts[index]] = struct{}{}
	}
	sort.Strings(parts)
	return parts, nil
}

func validObjectReference(reference string) bool {
	return len(reference) <= 253 && objectReferencePattern.MatchString(reference)
}

// DesiredState resolves one run against the immutable trusted template.
func (configuration ExecutionIntentConfig) DesiredState(run domain.AgentRun, profile string) (domain.AgentRunDesiredState, error) {
	image, ok := configuration.RunnerImages[run.Runtime]
	if !ok {
		return domain.AgentRunDesiredState{}, fmt.Errorf("no trusted runner image is configured for runtime")
	}
	return domain.AgentRunDesiredState{
		RunnerImage: image, Runtime: run.Runtime, ExecutionProfile: profile,
		TaskReference: run.PromptReference, TimeoutSeconds: run.TimeoutSeconds,
		MaxAttempts: run.MaxAttempts, InitialBackoffSeconds: 5, MaxBackoffSeconds: 300,
		RetryableFailureCategories: []domain.FailureCategory{domain.FailureTransientDependency, domain.FailureInternal},
		CPUMillis:                  run.CPUMillis, MemoryMiB: run.MemoryMiB, CPULimitMillis: run.CPUMillis,
		MemoryLimitMiB: run.MemoryMiB, WorkspaceSizeGiB: configuration.WorkspaceSizeGiB,
		StorageClassName: configuration.StorageClassName, WorkspaceRetentionPolicy: "Delete",
		NetworkProfile: "Isolated", ArtifactDestinationRef: configuration.ArtifactDestinationRef,
		ConfigurationRefs: configuration.ConfigurationRefs, SecretRefs: configuration.SecretRefs,
	}, nil
}
