package runner

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
)

const taskSchemaVersion = 1

type TaskEnvelope struct {
	SchemaVersion int        `json:"schemaVersion"`
	KeyID         string     `json:"keyId"`
	TenantID      string     `json:"tenantId"`
	ProjectID     string     `json:"projectId"`
	RunID         string     `json:"runId"`
	AttemptID     string     `json:"attemptId"`
	Attempt       int        `json:"attempt"`
	Template      string     `json:"template"`
	Tests         [][]string `json:"tests"`
}

type trustBundle struct {
	Keys map[string]string `json:"keys"`
}

func LoadSignedTask(config RuntimeConfig, secretRoot, configRoot string) (TaskEnvelope, error) {
	parsed, err := url.Parse(config.TaskRef)
	if err != nil || parsed.Scheme != "secret" || parsed.Host == "" || parsed.Path == "" {
		return TaskEnvelope{}, fmt.Errorf("taskRef must use secret://<name>/<key>")
	}
	secretName := parsed.Host
	key := filepath.Base(parsed.Path)
	if key == "." || key == "/" || parsed.Path != "/"+key || !slices.Contains(config.SecretRefs, secretName) || !referenceNamePattern.MatchString(secretName) {
		return TaskEnvelope{}, fmt.Errorf("task reference is not an approved secret projection")
	}
	path := filepath.Join(secretRoot, secretName, key)
	if filepath.Dir(path) != filepath.Join(secretRoot, secretName) {
		return TaskEnvelope{}, fmt.Errorf("task reference escapes secret mount")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return TaskEnvelope{}, fmt.Errorf("read task envelope: %w", err)
	}
	signatureText, err := os.ReadFile(path + ".sig")
	if err != nil {
		return TaskEnvelope{}, fmt.Errorf("read task signature: %w", err)
	}
	var task TaskEnvelope
	decoder := json.NewDecoder(bytesReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&task); err != nil {
		return TaskEnvelope{}, fmt.Errorf("decode task envelope: %w", err)
	}
	keyring, err := loadTrustBundle(config.ConfigurationRefs, configRoot)
	if err != nil {
		return TaskEnvelope{}, err
	}
	encoded, ok := keyring[task.KeyID]
	if !ok {
		return TaskEnvelope{}, fmt.Errorf("task signing key is not trusted")
	}
	publicKey, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return TaskEnvelope{}, fmt.Errorf("task signing key is invalid")
	}
	signature, err := base64.StdEncoding.DecodeString(string(signatureText))
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), body, signature) {
		return TaskEnvelope{}, fmt.Errorf("task signature is invalid")
	}
	if err := task.Validate(config); err != nil {
		return TaskEnvelope{}, err
	}
	return task, nil
}

func loadTrustBundle(references []string, root string) (map[string]string, error) {
	var bundle trustBundle
	found := false
	for _, reference := range references {
		contents, err := os.ReadFile(filepath.Join(root, reference, "task-trust.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read task trust bundle: %w", err)
		}
		if found {
			return nil, fmt.Errorf("exactly one task trust bundle is required")
		}
		decoder := json.NewDecoder(bytesReader(contents))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&bundle); err != nil {
			return nil, fmt.Errorf("decode task trust bundle: %w", err)
		}
		found = true
	}
	if !found || len(bundle.Keys) == 0 {
		return nil, fmt.Errorf("exactly one task trust bundle is required")
	}
	return bundle.Keys, nil
}

func (task TaskEnvelope) Validate(config RuntimeConfig) error {
	if task.SchemaVersion != taskSchemaVersion || task.KeyID == "" || task.Template != "python-basic" || len(task.Tests) == 0 {
		return fmt.Errorf("task envelope is invalid")
	}
	if task.TenantID != config.TenantID || task.ProjectID != config.ProjectID || task.RunID != config.RunID || task.AttemptID != config.AttemptID || task.Attempt != config.Attempt {
		return fmt.Errorf("task envelope identity does not match runtime config")
	}
	for _, command := range task.Tests {
		if len(command) == 0 {
			return fmt.Errorf("task contains an empty test command")
		}
	}
	return nil
}
