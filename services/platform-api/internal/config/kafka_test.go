package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadKafkaDefaultsAndOverrides(t *testing.T) {
	defaults, err := LoadKafka(testKafkaLookup(map[string]string{"AGENTFORGE_KAFKA_BROKERS": "127.0.0.1:19092"}))
	if err != nil || len(defaults.Brokers) != 1 || defaults.PublishTimeout != 10*time.Second || defaults.MaxAttempts != 3 || defaults.MaxMessageBytes != 1<<20 {
		t.Fatalf("defaults=%#v err=%v", defaults, err)
	}
	overrides, err := LoadKafka(testKafkaLookup(map[string]string{"AGENTFORGE_KAFKA_BROKERS": "kafka-a:9092, kafka-b:9092", "AGENTFORGE_KAFKA_CLIENT_ID": "relay", "AGENTFORGE_KAFKA_PUBLISH_TIMEOUT": "2s", "AGENTFORGE_KAFKA_MAX_ATTEMPTS": "2", "AGENTFORGE_KAFKA_MAX_MESSAGE_BYTES": "2048"}))
	if err != nil || len(overrides.Brokers) != 2 || overrides.ClientID != "relay" || overrides.PublishTimeout != 2*time.Second || overrides.MaxAttempts != 2 || overrides.MaxMessageBytes != 2048 {
		t.Fatalf("overrides=%#v err=%v", overrides, err)
	}
}

func TestLoadKafkaRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{key: "AGENTFORGE_KAFKA_BROKERS", value: "missing-port"},
		{key: "AGENTFORGE_KAFKA_CLIENT_ID", value: ""},
		{key: "AGENTFORGE_KAFKA_PUBLISH_TIMEOUT", value: "31s"},
		{key: "AGENTFORGE_KAFKA_MAX_ATTEMPTS", value: "4"},
		{key: "AGENTFORGE_KAFKA_MAX_MESSAGE_BYTES", value: "12"},
	} {
		t.Run(test.key, func(t *testing.T) {
			values := map[string]string{"AGENTFORGE_KAFKA_BROKERS": "127.0.0.1:19092", test.key: test.value}
			_, err := LoadKafka(testKafkaLookup(values))
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestKafkaConfigValidateRejectsDirectZeroValue(t *testing.T) {
	if err := (KafkaConfig{}).Validate(); err == nil {
		t.Fatal("zero KafkaConfig passed validation")
	}
}

func testKafkaLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
