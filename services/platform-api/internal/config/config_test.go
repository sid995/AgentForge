package config

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLoadDefaults(t *testing.T) {
	config, err := Load(testLookup(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.HTTPAddress != ":8080" || config.Environment != "development" {
		t.Fatalf("Load() config = %#v", config)
	}
	if config.ShutdownTimeout != 10*time.Second || config.RequestTimeout != 30*time.Second || config.MaxRequestBodyBytes != 1<<20 {
		t.Fatalf("Load() config = %#v", config)
	}
	if config.Database.MaxConns != 10 || config.Database.MaxIdleConns != 5 || config.Database.AcquireTimeout != 5*time.Second {
		t.Fatalf("Load() database config = %#v", config.Database)
	}
}

func TestLoadDevelopmentIdentity(t *testing.T) {
	tenantID := uuid.Must(uuid.NewV7())
	configuration, err := Load(testLookup(map[string]string{
		"AGENTFORGE_DEVELOPMENT_IDENTITY_TOKEN":     "local-token",
		"AGENTFORGE_DEVELOPMENT_IDENTITY_TENANT_ID": tenantID.String(),
		"AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT":   "developer@example.test",
		"AGENTFORGE_DEVELOPMENT_IDENTITY_ROLE":      "developer",
	}))
	if err != nil || configuration.DevelopmentIdentity == nil || configuration.DevelopmentIdentity.TenantID != tenantID {
		t.Fatalf("Load() development identity=%#v error=%v", configuration.DevelopmentIdentity, err)
	}
}

func TestLoadRejectsDevelopmentIdentityOutsideLocalEnvironment(t *testing.T) {
	_, err := Load(testLookup(map[string]string{
		"AGENTFORGE_ENVIRONMENT":                    "production",
		"AGENTFORGE_DEVELOPMENT_IDENTITY_TOKEN":     "local-token",
		"AGENTFORGE_DEVELOPMENT_IDENTITY_TENANT_ID": uuid.Must(uuid.NewV7()).String(),
		"AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT":   "developer@example.test",
		"AGENTFORGE_DEVELOPMENT_IDENTITY_ROLE":      "developer",
	}))
	if err == nil || !strings.Contains(err.Error(), "AGENTFORGE_DEVELOPMENT_IDENTITY_TOKEN") {
		t.Fatalf("Load() error=%v, want development identity validation", err)
	}
}

func TestLoadOverrides(t *testing.T) {
	values := map[string]string{
		"AGENTFORGE_HTTP_ADDR":                  "127.0.0.1:9000",
		"AGENTFORGE_ENVIRONMENT":                "test",
		"AGENTFORGE_SHUTDOWN_TIMEOUT":           "3s",
		"AGENTFORGE_REQUEST_TIMEOUT":            "4s",
		"AGENTFORGE_MAX_REQUEST_BODY_BYTES":     "512",
		"AGENTFORGE_DATABASE_URL":               "postgres://user:password@localhost:5432/agentforge?sslmode=disable",
		"AGENTFORGE_DATABASE_MAX_CONNS":         "12",
		"AGENTFORGE_DATABASE_MAX_IDLE_CONNS":    "2",
		"AGENTFORGE_DATABASE_MAX_CONN_LIFETIME": "4m",
		"AGENTFORGE_DATABASE_ACQUIRE_TIMEOUT":   "3s",
		"AGENTFORGE_DATABASE_CONNECT_TIMEOUT":   "2s",
	}
	config, err := Load(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.HTTPAddress != "127.0.0.1:9000" || config.Environment != "test" || config.ShutdownTimeout != 3*time.Second || config.RequestTimeout != 4*time.Second || config.MaxRequestBodyBytes != 512 || config.Database.MaxConns != 12 || config.Database.MaxIdleConns != 2 || config.Database.MaxConnLifetime != 4*time.Minute || config.Database.AcquireTimeout != 3*time.Second || config.Database.ConnectTimeout != 2*time.Second {
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
		{name: "database URL", key: "AGENTFORGE_DATABASE_URL", value: "mysql://localhost/agentforge"},
		{name: "database URL without user", key: "AGENTFORGE_DATABASE_URL", value: "postgres://localhost/agentforge"},
		{name: "database max connections", key: "AGENTFORGE_DATABASE_MAX_CONNS", value: "0"},
		{name: "database max idle connections", key: "AGENTFORGE_DATABASE_MAX_IDLE_CONNS", value: "-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(testLookup(map[string]string{test.key: test.value}))
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error = %v, want error naming %s", err, test.key)
			}
		})
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	_, err := Load(func(string) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), "AGENTFORGE_DATABASE_URL") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsDatabasePoolBounds(t *testing.T) {
	_, err := Load(testLookup(map[string]string{
		"AGENTFORGE_DATABASE_MAX_CONNS":      "2",
		"AGENTFORGE_DATABASE_MAX_IDLE_CONNS": "3",
	}))
	if err == nil || !strings.Contains(err.Error(), "AGENTFORGE_DATABASE_MAX_IDLE_CONNS") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadClampsDefaultIdleConnectionsToMaximum(t *testing.T) {
	configuration, err := Load(testLookup(map[string]string{"AGENTFORGE_DATABASE_MAX_CONNS": "2"}))
	if err != nil || configuration.Database.MaxIdleConns != 2 {
		t.Fatalf("Load() config = %#v, error = %v", configuration.Database, err)
	}
}

func testLookup(values map[string]string) LookupEnv {
	defaults := map[string]string{
		"AGENTFORGE_DATABASE_URL": "postgres://user:password@localhost:5432/agentforge?sslmode=disable",
	}
	for key, value := range values {
		defaults[key] = value
	}
	return func(name string) (string, bool) {
		value, ok := defaults[name]
		return value, ok
	}
}
