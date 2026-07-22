package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultKafkaClientID        = "agentforge-platform-api"
	defaultKafkaPublishTimeout  = 10 * time.Second
	defaultKafkaMaxAttempts     = 3
	defaultKafkaMaxMessageBytes = 1 << 20
)

// KafkaConfig contains provider-neutral Kafka-compatible client settings.
type KafkaConfig struct {
	Brokers         []string
	ClientID        string
	PublishTimeout  time.Duration
	MaxAttempts     int
	MaxMessageBytes int
}

// LoadKafka reads the independently deployable event-client configuration.
func LoadKafka(lookup LookupEnv) (KafkaConfig, error) {
	configuration := KafkaConfig{ClientID: defaultKafkaClientID, PublishTimeout: defaultKafkaPublishTimeout, MaxAttempts: defaultKafkaMaxAttempts, MaxMessageBytes: defaultKafkaMaxMessageBytes}
	value, ok := lookup("AGENTFORGE_KAFKA_BROKERS")
	if !ok || strings.TrimSpace(value) == "" {
		return KafkaConfig{}, fmt.Errorf("AGENTFORGE_KAFKA_BROKERS must be configured")
	}
	for _, broker := range strings.Split(value, ",") {
		broker = strings.TrimSpace(broker)
		if _, _, err := net.SplitHostPort(broker); err != nil {
			return KafkaConfig{}, fmt.Errorf("AGENTFORGE_KAFKA_BROKERS must contain comma-separated host:port entries")
		}
		configuration.Brokers = append(configuration.Brokers, broker)
	}
	if value, ok := lookup("AGENTFORGE_KAFKA_CLIENT_ID"); ok {
		configuration.ClientID = strings.TrimSpace(value)
	}
	if configuration.ClientID == "" || len(configuration.ClientID) > 128 {
		return KafkaConfig{}, fmt.Errorf("AGENTFORGE_KAFKA_CLIENT_ID must contain 1 to 128 characters")
	}
	var err error
	if configuration.PublishTimeout, err = duration(lookup, "AGENTFORGE_KAFKA_PUBLISH_TIMEOUT", configuration.PublishTimeout); err != nil {
		return KafkaConfig{}, err
	}
	if configuration.PublishTimeout < time.Second || configuration.PublishTimeout > 30*time.Second {
		return KafkaConfig{}, fmt.Errorf("AGENTFORGE_KAFKA_PUBLISH_TIMEOUT must be between 1s and 30s")
	}
	if configuration.MaxAttempts, err = boundedInt(lookup, "AGENTFORGE_KAFKA_MAX_ATTEMPTS", configuration.MaxAttempts, 1, 3); err != nil {
		return KafkaConfig{}, err
	}
	if configuration.MaxMessageBytes, err = boundedInt(lookup, "AGENTFORGE_KAFKA_MAX_MESSAGE_BYTES", configuration.MaxMessageBytes, 1024, 10<<20); err != nil {
		return KafkaConfig{}, err
	}
	if err := configuration.Validate(); err != nil {
		return KafkaConfig{}, err
	}
	return configuration, nil
}

// Validate protects adapter invariants even when configuration is constructed directly.
func (configuration KafkaConfig) Validate() error {
	if len(configuration.Brokers) == 0 {
		return fmt.Errorf("AGENTFORGE_KAFKA_BROKERS must be configured")
	}
	for _, broker := range configuration.Brokers {
		if _, _, err := net.SplitHostPort(strings.TrimSpace(broker)); err != nil {
			return fmt.Errorf("AGENTFORGE_KAFKA_BROKERS must contain comma-separated host:port entries")
		}
	}
	if strings.TrimSpace(configuration.ClientID) == "" || len(configuration.ClientID) > 128 {
		return fmt.Errorf("AGENTFORGE_KAFKA_CLIENT_ID must contain 1 to 128 characters")
	}
	if configuration.PublishTimeout < time.Second || configuration.PublishTimeout > 30*time.Second {
		return fmt.Errorf("AGENTFORGE_KAFKA_PUBLISH_TIMEOUT must be between 1s and 30s")
	}
	if configuration.MaxAttempts < 1 || configuration.MaxAttempts > 3 {
		return fmt.Errorf("AGENTFORGE_KAFKA_MAX_ATTEMPTS must be between 1 and 3")
	}
	if configuration.MaxMessageBytes < 1024 || configuration.MaxMessageBytes > 10<<20 {
		return fmt.Errorf("AGENTFORGE_KAFKA_MAX_MESSAGE_BYTES must be between 1024 and 10485760")
	}
	return nil
}

func boundedInt(lookup LookupEnv, name string, fallback, minimum, maximum int) (int, error) {
	value, ok := lookup(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}
