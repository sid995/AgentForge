package config

import (
	"fmt"
	"strings"
)

const defaultSchedulerStrategy = "least-loaded"

// SchedulerStrategy returns the configured deterministic cluster strategy.
func SchedulerStrategy(lookup LookupEnv) (string, error) {
	value := defaultSchedulerStrategy
	if configured, ok := lookup("AGENTFORGE_SCHEDULER_STRATEGY"); ok {
		value = strings.TrimSpace(configured)
	}
	if value != "least-loaded" && value != "region-affinity" {
		return "", fmt.Errorf("AGENTFORGE_SCHEDULER_STRATEGY must be least-loaded or region-affinity")
	}
	return value, nil
}
