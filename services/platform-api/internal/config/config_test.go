package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	config, err := Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.HTTPAddress != ":8080" || config.Environment != "development" {
		t.Fatalf("Load() config = %#v", config)
	}
	if config.ShutdownTimeout != 10*time.Second || config.RequestTimeout != 30*time.Second || config.MaxRequestBodyBytes != 1<<20 {
		t.Fatalf("Load() config = %#v", config)
	}
}

func TestLoadOverrides(t *testing.T) {
	values := map[string]string{
		"AGENTFORGE_HTTP_ADDR":              "127.0.0.1:9000",
		"AGENTFORGE_ENVIRONMENT":            "test",
		"AGENTFORGE_SHUTDOWN_TIMEOUT":       "3s",
		"AGENTFORGE_REQUEST_TIMEOUT":        "4s",
		"AGENTFORGE_MAX_REQUEST_BODY_BYTES": "512",
	}
	config, err := Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.HTTPAddress != "127.0.0.1:9000" || config.Environment != "test" || config.ShutdownTimeout != 3*time.Second || config.RequestTimeout != 4*time.Second || config.MaxRequestBodyBytes != 512 {
		t.Fatalf("Load() config = %#v", config)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "address", key: "AGENTFORGE_HTTP_ADDR", value: "invalid"},
		{name: "environment", key: "AGENTFORGE_ENVIRONMENT", value: ""},
		{name: "shutdown timeout", key: "AGENTFORGE_SHUTDOWN_TIMEOUT", value: "0s"},
		{name: "request timeout", key: "AGENTFORGE_REQUEST_TIMEOUT", value: "invalid"},
		{name: "body limit", key: "AGENTFORGE_MAX_REQUEST_BODY_BYTES", value: "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(func(name string) (string, bool) {
				if name == test.key {
					return test.value, true
				}
				return "", false
			})
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error = %v, want error naming %s", err, test.key)
			}
		})
	}
}
