package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddress       = ":8080"
	defaultEnvironment       = "development"
	defaultShutdownTimeout   = 10 * time.Second
	defaultRequestTimeout    = 30 * time.Second
	defaultMaxRequestBodyLen = int64(1 << 20)
	maxEnvironmentLength     = 64
)

// Config contains the bounded process configuration for Platform API.
type Config struct {
	HTTPAddress         string
	Environment         string
	ShutdownTimeout     time.Duration
	RequestTimeout      time.Duration
	MaxRequestBodyBytes int64
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

	return config, nil
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
