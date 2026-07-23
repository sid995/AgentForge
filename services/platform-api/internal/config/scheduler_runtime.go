package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type SchedulerConfig struct {
	HTTPAddress       string
	Environment       string
	Database          DatabaseConfig
	ShutdownTimeout   time.Duration
	Owner             string
	PollInterval      time.Duration
	LeaseDuration     time.Duration
	BatchSize         int
	WorkerCount       int
	WorkQueueSize     int
	AgingInterval     time.Duration
	MaximumAgingBoost int
	ClusterFreshness  time.Duration
	ReservationTTL    time.Duration
	DeferralDuration  time.Duration
	Strategy          string
	KafkaHintsEnabled bool
	KafkaGroup        string
	ExecutionIntent   ExecutionIntentConfig
}

func LoadScheduler(lookup LookupEnv) (SchedulerConfig, error) {
	configuration := SchedulerConfig{HTTPAddress: ":8081", Environment: defaultEnvironment, ShutdownTimeout: defaultShutdownTimeout, PollInterval: 2 * time.Second, LeaseDuration: 30 * time.Second, BatchSize: 10, WorkerCount: 4, WorkQueueSize: 16, AgingInterval: time.Minute, MaximumAgingBoost: 20, ClusterFreshness: 30 * time.Second, ReservationTTL: 5 * time.Minute, DeferralDuration: 30 * time.Second, KafkaGroup: "agentforge-scheduler-hints-v1", Database: DatabaseConfig{MaxConns: defaultDatabaseMaxConns, MaxIdleConns: defaultDatabaseMaxIdle, MaxConnLifetime: defaultDatabaseLifetime, AcquireTimeout: defaultDatabaseAcquire, ConnectTimeout: defaultDatabaseConnect}}
	if value, ok := lookup("AGENTFORGE_SCHEDULER_HTTP_ADDR"); ok {
		configuration.HTTPAddress = strings.TrimSpace(value)
	}
	if _, _, err := net.SplitHostPort(configuration.HTTPAddress); err != nil {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_HTTP_ADDR must be a valid host:port: %w", err)
	}
	if value, ok := lookup("AGENTFORGE_ENVIRONMENT"); ok {
		configuration.Environment = strings.TrimSpace(value)
	}
	if configuration.Environment == "" || len(configuration.Environment) > maxEnvironmentLength {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_ENVIRONMENT must contain 1 to %d characters", maxEnvironmentLength)
	}
	if value, ok := lookup("AGENTFORGE_SCHEDULER_OWNER"); ok {
		configuration.Owner = strings.TrimSpace(value)
	}
	if len(configuration.Owner) > 160 {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_OWNER must not exceed 160 characters")
	}
	var err error
	if configuration.Database, err = database(lookup, configuration.Database); err != nil {
		return SchedulerConfig{}, err
	}
	for name, target := range map[string]*time.Duration{"AGENTFORGE_SCHEDULER_POLL_INTERVAL": &configuration.PollInterval, "AGENTFORGE_SCHEDULER_LEASE_DURATION": &configuration.LeaseDuration, "AGENTFORGE_SCHEDULER_AGING_INTERVAL": &configuration.AgingInterval, "AGENTFORGE_SCHEDULER_CLUSTER_FRESHNESS": &configuration.ClusterFreshness, "AGENTFORGE_SCHEDULER_RESERVATION_TTL": &configuration.ReservationTTL, "AGENTFORGE_SCHEDULER_DEFERRAL_DURATION": &configuration.DeferralDuration, "AGENTFORGE_SHUTDOWN_TIMEOUT": &configuration.ShutdownTimeout} {
		if *target, err = duration(lookup, name, *target); err != nil {
			return SchedulerConfig{}, err
		}
	}
	if configuration.LeaseDuration <= configuration.PollInterval {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_LEASE_DURATION must exceed poll interval")
	}
	for name, input := range map[string]struct {
		target           *int
		minimum, maximum int
	}{"AGENTFORGE_SCHEDULER_BATCH_SIZE": {&configuration.BatchSize, 1, 100}, "AGENTFORGE_SCHEDULER_WORKERS": {&configuration.WorkerCount, 1, 64}, "AGENTFORGE_SCHEDULER_WORK_QUEUE_SIZE": {&configuration.WorkQueueSize, 1, 1000}, "AGENTFORGE_SCHEDULER_MAX_AGING_BOOST": {&configuration.MaximumAgingBoost, 0, 100}} {
		if *input.target, err = boundedInt(lookup, name, *input.target, input.minimum, input.maximum); err != nil {
			return SchedulerConfig{}, err
		}
	}
	if configuration.WorkQueueSize < configuration.WorkerCount {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_WORK_QUEUE_SIZE must be at least worker count")
	}
	if configuration.Strategy, err = SchedulerStrategy(lookup); err != nil {
		return SchedulerConfig{}, err
	}
	if value, ok := lookup("AGENTFORGE_SCHEDULER_KAFKA_HINTS_ENABLED"); ok {
		configuration.KafkaHintsEnabled, err = strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_KAFKA_HINTS_ENABLED must be true or false")
		}
	}
	if value, ok := lookup("AGENTFORGE_SCHEDULER_KAFKA_GROUP"); ok {
		configuration.KafkaGroup = strings.TrimSpace(value)
	}
	if configuration.KafkaGroup == "" || len(configuration.KafkaGroup) > 160 {
		return SchedulerConfig{}, fmt.Errorf("AGENTFORGE_SCHEDULER_KAFKA_GROUP must contain 1 to 160 characters")
	}
	if configuration.ExecutionIntent, err = loadExecutionIntent(lookup); err != nil {
		return SchedulerConfig{}, err
	}
	return configuration, nil
}
