package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

const runtimeSchemaVersion = 1

var referenceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

type RuntimeConfig struct {
	SchemaVersion          int      `json:"schemaVersion"`
	TenantID               string   `json:"tenantId"`
	ProjectID              string   `json:"projectId"`
	RunID                  string   `json:"runId"`
	AttemptID              string   `json:"attemptId"`
	Attempt                int      `json:"attempt"`
	TaskRef                string   `json:"taskRef"`
	TimeoutSeconds         int      `json:"timeoutSeconds"`
	ConfigurationRefs      []string `json:"configurationRefs"`
	SecretRefs             []string `json:"secretRefs"`
	ArtifactDestinationRef string   `json:"artifactDestinationRef"`
}

func LoadRuntimeConfig(path string) (RuntimeConfig, error) {
	if path == "" {
		return RuntimeConfig{}, fmt.Errorf("AGENTFORGE_RUN_CONFIG is required")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read runtime config: %w", err)
	}
	decoder := json.NewDecoder(bytesReader(contents))
	decoder.DisallowUnknownFields()
	var config RuntimeConfig
	if err := decoder.Decode(&config); err != nil {
		return RuntimeConfig{}, fmt.Errorf("decode runtime config: %w", err)
	}
	if config.SchemaVersion != runtimeSchemaVersion || config.TimeoutSeconds < 1 || config.Attempt < 1 {
		return RuntimeConfig{}, fmt.Errorf("runtime config has unsupported schema or bounds")
	}
	for _, value := range []string{config.TenantID, config.ProjectID, config.RunID, config.AttemptID, config.ArtifactDestinationRef} {
		if value == "" {
			return RuntimeConfig{}, fmt.Errorf("runtime config has missing identity")
		}
	}
	if err := validateReferences(config.ConfigurationRefs); err != nil {
		return RuntimeConfig{}, fmt.Errorf("configuration refs: %w", err)
	}
	if err := validateReferences(config.SecretRefs); err != nil {
		return RuntimeConfig{}, fmt.Errorf("secret refs: %w", err)
	}
	return config, nil
}

func validateReferences(references []string) error {
	if !slices.IsSorted(references) || len(slices.Compact(slices.Clone(references))) != len(references) {
		return fmt.Errorf("references must be sorted and unique")
	}
	for _, reference := range references {
		if !referenceNamePattern.MatchString(reference) || filepath.Base(reference) != reference {
			return fmt.Errorf("invalid reference")
		}
	}
	return nil
}
