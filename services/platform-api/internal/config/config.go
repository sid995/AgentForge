package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultHTTPAddress       = ":8080"
	defaultEnvironment       = "development"
	defaultShutdownTimeout   = 10 * time.Second
	defaultRequestTimeout    = 30 * time.Second
	defaultMaxRequestBodyLen = int64(1 << 20)
	maxEnvironmentLength     = 64
	defaultDatabaseMaxConns  = int32(10)
	defaultDatabaseMaxIdle   = 5
	defaultDatabaseLifetime  = 30 * time.Minute
	defaultDatabaseAcquire   = 5 * time.Second
	defaultDatabaseConnect   = 5 * time.Second
)

// Config contains the bounded process configuration for Platform API.
type Config struct {
	HTTPAddress         string
	Environment         string
	ShutdownTimeout     time.Duration
	RequestTimeout      time.Duration
	MaxRequestBodyBytes int64
	Database            DatabaseConfig
	DevelopmentIdentity *DevelopmentIdentityConfig
	ArtifactStorage     *ArtifactStorageConfig
}

// DevelopmentIdentityConfig is an explicitly local-only opaque bearer identity.
// It is replaced by OIDC and membership lookup in Phase 12.
type DevelopmentIdentityConfig struct {
	Token    string
	TenantID uuid.UUID
	Subject  string
	Role     string
}

// DatabaseConfig contains the bounded PostgreSQL connection-pool settings.
type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MaxIdleConns    int
	MaxConnLifetime time.Duration
	AcquireTimeout  time.Duration
	ConnectTimeout  time.Duration
}

// LookupEnv reads an environment variable. It is injected to keep validation
// deterministic in tests.
type LookupEnv func(string) (string, bool)

// Load reads Platform API configuration from environment variables.
func Load(lookup LookupEnv) (Config, error) {
	config := Config{
		HTTPAddress:         defaultHTTPAddress,
		Environment:         defaultEnvironment,
		ShutdownTimeout:     defaultShutdownTimeout,
		RequestTimeout:      defaultRequestTimeout,
		MaxRequestBodyBytes: defaultMaxRequestBodyLen,
		Database: DatabaseConfig{
			MaxConns:        defaultDatabaseMaxConns,
			MaxIdleConns:    defaultDatabaseMaxIdle,
			MaxConnLifetime: defaultDatabaseLifetime,
			AcquireTimeout:  defaultDatabaseAcquire,
			ConnectTimeout:  defaultDatabaseConnect,
		},
	}

	if value, ok := lookup("AGENTFORGE_HTTP_ADDR"); ok {
		config.HTTPAddress = strings.TrimSpace(value)
	}
	if _, _, err := net.SplitHostPort(config.HTTPAddress); err != nil {
		return Config{}, fmt.Errorf("AGENTFORGE_HTTP_ADDR must be a valid host:port: %w", err)
	}

	if value, ok := lookup("AGENTFORGE_ENVIRONMENT"); ok {
		config.Environment = strings.TrimSpace(value)
	}
	if config.Environment == "" || len(config.Environment) > maxEnvironmentLength {
		return Config{}, fmt.Errorf("AGENTFORGE_ENVIRONMENT must contain 1 to %d characters", maxEnvironmentLength)
	}

	var err error
	if config.ShutdownTimeout, err = duration(lookup, "AGENTFORGE_SHUTDOWN_TIMEOUT", config.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if config.RequestTimeout, err = duration(lookup, "AGENTFORGE_REQUEST_TIMEOUT", config.RequestTimeout); err != nil {
		return Config{}, err
	}
	if config.MaxRequestBodyBytes, err = positiveInt64(lookup, "AGENTFORGE_MAX_REQUEST_BODY_BYTES", config.MaxRequestBodyBytes); err != nil {
		return Config{}, err
	}
	if config.Database, err = database(lookup, config.Database); err != nil {
		return Config{}, err
	}
	if config.DevelopmentIdentity, err = developmentIdentity(lookup, config.Environment); err != nil {
		return Config{}, err
	}
	if config.ArtifactStorage, err = artifactStorage(lookup); err != nil {
		return Config{}, err
	}

	return config, nil
}

func developmentIdentity(lookup LookupEnv, environment string) (*DevelopmentIdentityConfig, error) {
	const tokenKey = "AGENTFORGE_DEVELOPMENT_IDENTITY_TOKEN"
	values := map[string]string{}
	keys := []string{tokenKey, "AGENTFORGE_DEVELOPMENT_IDENTITY_TENANT_ID", "AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT", "AGENTFORGE_DEVELOPMENT_IDENTITY_ROLE"}
	configured := false
	for _, key := range keys {
		if value, ok := lookup(key); ok {
			values[key] = strings.TrimSpace(value)
			configured = true
		}
	}
	if !configured {
		return nil, nil
	}
	if environment != "development" && environment != "test" {
		return nil, fmt.Errorf("%s is permitted only in development or test", tokenKey)
	}
	for _, key := range keys {
		if values[key] == "" {
			return nil, fmt.Errorf("%s must be configured with all development identity fields", tokenKey)
		}
	}
	tenantID, err := uuid.Parse(values["AGENTFORGE_DEVELOPMENT_IDENTITY_TENANT_ID"])
	if err != nil || tenantID == uuid.Nil {
		return nil, fmt.Errorf("AGENTFORGE_DEVELOPMENT_IDENTITY_TENANT_ID must be a UUID")
	}
	if len(values["AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT"]) > 255 {
		return nil, fmt.Errorf("AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT must contain at most 255 characters")
	}
	role := values["AGENTFORGE_DEVELOPMENT_IDENTITY_ROLE"]
	if role != "developer" && role != "project-administrator" {
		return nil, fmt.Errorf("AGENTFORGE_DEVELOPMENT_IDENTITY_ROLE must be developer or project-administrator")
	}
	return &DevelopmentIdentityConfig{Token: values[tokenKey], TenantID: tenantID, Subject: values["AGENTFORGE_DEVELOPMENT_IDENTITY_SUBJECT"], Role: role}, nil
}

func database(lookup LookupEnv, defaults DatabaseConfig) (DatabaseConfig, error) {
	value, ok := lookup("AGENTFORGE_DATABASE_URL")
	if !ok || strings.TrimSpace(value) == "" {
		return DatabaseConfig{}, fmt.Errorf("AGENTFORGE_DATABASE_URL must be configured")
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || parsed.User == nil || parsed.User.Username() == "" || strings.Trim(parsed.Path, "/") == "" {
		return DatabaseConfig{}, fmt.Errorf("AGENTFORGE_DATABASE_URL must be a valid PostgreSQL URL")
	}
	defaults.URL = parsed.String()

	max, err := positiveInt32(lookup, "AGENTFORGE_DATABASE_MAX_CONNS", defaults.MaxConns)
	if err != nil {
		return DatabaseConfig{}, err
	}
	defaults.MaxConns = max
	if idle, exists := lookup("AGENTFORGE_DATABASE_MAX_IDLE_CONNS"); exists {
		parsedIdle, parseErr := strconv.ParseInt(strings.TrimSpace(idle), 10, 32)
		if parseErr != nil || parsedIdle < 0 {
			return DatabaseConfig{}, fmt.Errorf("AGENTFORGE_DATABASE_MAX_IDLE_CONNS must be a non-negative integer")
		}
		defaults.MaxIdleConns = int(parsedIdle)
	} else if int32(defaults.MaxIdleConns) > defaults.MaxConns {
		defaults.MaxIdleConns = int(defaults.MaxConns)
	}
	if int32(defaults.MaxIdleConns) > defaults.MaxConns {
		return DatabaseConfig{}, fmt.Errorf("AGENTFORGE_DATABASE_MAX_IDLE_CONNS must not exceed AGENTFORGE_DATABASE_MAX_CONNS")
	}
	if defaults.MaxConnLifetime, err = duration(lookup, "AGENTFORGE_DATABASE_MAX_CONN_LIFETIME", defaults.MaxConnLifetime); err != nil {
		return DatabaseConfig{}, err
	}
	if defaults.AcquireTimeout, err = duration(lookup, "AGENTFORGE_DATABASE_ACQUIRE_TIMEOUT", defaults.AcquireTimeout); err != nil {
		return DatabaseConfig{}, err
	}
	if defaults.ConnectTimeout, err = duration(lookup, "AGENTFORGE_DATABASE_CONNECT_TIMEOUT", defaults.ConnectTimeout); err != nil {
		return DatabaseConfig{}, err
	}
	return defaults, nil
}

func duration(lookup LookupEnv, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(name)
	if !ok {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func positiveInt64(lookup LookupEnv, name string, fallback int64) (int64, error) {
	value, ok := lookup(name)
	if !ok {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func positiveInt32(lookup LookupEnv, name string, fallback int32) (int32, error) {
	value, ok := lookup(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return int32(parsed), nil
}
