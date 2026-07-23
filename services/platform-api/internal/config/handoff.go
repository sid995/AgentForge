package config

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var handoffClusterIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// HandoffConfig configures the isolated Scheduler-event-to-CR consumer.
type HandoffConfig struct {
	HTTPAddress        string
	Environment        string
	Database           DatabaseConfig
	ShutdownTimeout    time.Duration
	KafkaGroup         string
	KubeconfigPath     string
	ClusterContexts    map[string]string
	QuarantineTenantID string
}

func LoadHandoff(lookup LookupEnv) (HandoffConfig, error) {
	configuration := HandoffConfig{
		HTTPAddress: ":8082", Environment: defaultEnvironment, ShutdownTimeout: defaultShutdownTimeout,
		KafkaGroup: "agentforge-agentrun-handoff-v1",
		Database:   DatabaseConfig{MaxConns: defaultDatabaseMaxConns, MaxIdleConns: defaultDatabaseMaxIdle, MaxConnLifetime: defaultDatabaseLifetime, AcquireTimeout: defaultDatabaseAcquire, ConnectTimeout: defaultDatabaseConnect},
	}
	if value, ok := lookup("AGENTFORGE_HANDOFF_HTTP_ADDR"); ok {
		configuration.HTTPAddress = strings.TrimSpace(value)
	}
	if _, _, err := net.SplitHostPort(configuration.HTTPAddress); err != nil {
		return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_HTTP_ADDR must be a valid host:port: %w", err)
	}
	if value, ok := lookup("AGENTFORGE_ENVIRONMENT"); ok {
		configuration.Environment = strings.TrimSpace(value)
	}
	var err error
	if configuration.Database, err = database(lookup, configuration.Database); err != nil {
		return HandoffConfig{}, err
	}
	if configuration.ShutdownTimeout, err = duration(lookup, "AGENTFORGE_SHUTDOWN_TIMEOUT", configuration.ShutdownTimeout); err != nil {
		return HandoffConfig{}, err
	}
	if value, ok := lookup("AGENTFORGE_HANDOFF_KAFKA_GROUP"); ok {
		configuration.KafkaGroup = strings.TrimSpace(value)
	}
	if configuration.KafkaGroup == "" || len(configuration.KafkaGroup) > 160 {
		return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_KAFKA_GROUP must contain 1 to 160 characters")
	}
	value, ok := lookup("AGENTFORGE_HANDOFF_KUBECONFIG")
	configuration.KubeconfigPath = strings.TrimSpace(value)
	if !ok || configuration.KubeconfigPath == "" {
		return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_KUBECONFIG is required")
	}
	value, ok = lookup("AGENTFORGE_HANDOFF_CLUSTER_CONTEXTS")
	if !ok || json.Unmarshal([]byte(value), &configuration.ClusterContexts) != nil || len(configuration.ClusterContexts) == 0 {
		return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_CLUSTER_CONTEXTS must be a non-empty JSON object")
	}
	for clusterID, contextName := range configuration.ClusterContexts {
		if !handoffClusterIDPattern.MatchString(clusterID) || strings.TrimSpace(contextName) == "" || len(contextName) > 253 {
			return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_CLUSTER_CONTEXTS contains invalid cluster or context")
		}
	}
	value, ok = lookup("AGENTFORGE_HANDOFF_QUARANTINE_TENANT_ID")
	configuration.QuarantineTenantID = strings.TrimSpace(value)
	quarantineID, parseErr := uuid.Parse(configuration.QuarantineTenantID)
	if !ok || parseErr != nil || quarantineID.Version() != 7 {
		return HandoffConfig{}, fmt.Errorf("AGENTFORGE_HANDOFF_QUARANTINE_TENANT_ID must be a UUIDv7")
	}
	return configuration, nil
}
