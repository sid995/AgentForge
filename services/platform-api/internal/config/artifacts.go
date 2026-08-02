package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ArtifactStorageConfig contains server-owned S3-compatible storage credentials.
// It is intentionally absent unless every required setting is configured.
type ArtifactStorageConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Prefix    string
	Secure    bool
}

func artifactStorage(lookup LookupEnv) (*ArtifactStorageConfig, error) {
	const endpointKey = "AGENTFORGE_ARTIFACT_S3_ENDPOINT"
	keys := []string{endpointKey, "AGENTFORGE_ARTIFACT_S3_ACCESS_KEY", "AGENTFORGE_ARTIFACT_S3_SECRET_KEY", "AGENTFORGE_ARTIFACT_S3_BUCKET", "AGENTFORGE_ARTIFACT_S3_PREFIX", "AGENTFORGE_ARTIFACT_S3_SECURE"}
	values := make(map[string]string, len(keys))
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
	for _, key := range keys[:4] {
		if values[key] == "" {
			return nil, fmt.Errorf("%s must be configured with all artifact S3 settings", endpointKey)
		}
	}
	if len(values[endpointKey]) > 255 || strings.Contains(values[endpointKey], "://") || strings.ContainsAny(values[endpointKey], "/?#@") {
		return nil, fmt.Errorf("%s must be a host[:port] without a URL scheme", endpointKey)
	}
	if len(values["AGENTFORGE_ARTIFACT_S3_BUCKET"]) > 63 || strings.ContainsAny(values["AGENTFORGE_ARTIFACT_S3_BUCKET"], "/\\") {
		return nil, fmt.Errorf("AGENTFORGE_ARTIFACT_S3_BUCKET is invalid")
	}
	secure := true
	if values["AGENTFORGE_ARTIFACT_S3_SECURE"] != "" {
		parsed, err := strconv.ParseBool(values["AGENTFORGE_ARTIFACT_S3_SECURE"])
		if err != nil {
			return nil, fmt.Errorf("AGENTFORGE_ARTIFACT_S3_SECURE must be a boolean")
		}
		secure = parsed
	}
	return &ArtifactStorageConfig{Endpoint: values[endpointKey], AccessKey: values["AGENTFORGE_ARTIFACT_S3_ACCESS_KEY"], SecretKey: values["AGENTFORGE_ARTIFACT_S3_SECRET_KEY"], Bucket: values["AGENTFORGE_ARTIFACT_S3_BUCKET"], Prefix: values["AGENTFORGE_ARTIFACT_S3_PREFIX"], Secure: secure}, nil
}
