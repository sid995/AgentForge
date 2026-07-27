package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	ExitSuccess       = 0
	ExitConfig        = 2
	ExitPolicy        = 3
	ExitCommand       = 4
	ExitDeadline      = 5
	ExitCancelled     = 6
	ExitArtifacts     = 7
	ExitInternal      = 8
	defaultSecretRoot = "/var/run/agentforge/secrets"
	defaultConfigRoot = "/etc/agentforge/config"
)

type Options struct {
	ConfigPath, Workspace                                          string
	LogWriter                                                      io.Writer
	SecretRoot, ConfigRoot, ArtifactConfigPath, TerminationLogPath string
}

func Run(parent context.Context, options Options) (int, error) {
	if options.Workspace == "" {
		return ExitConfig, fmt.Errorf("AGENTFORGE_WORKSPACE is required")
	}
	if options.LogWriter == nil {
		options.LogWriter = io.Discard
	}
	if options.SecretRoot == "" {
		options.SecretRoot = defaultSecretRoot
	}
	if options.ConfigRoot == "" {
		options.ConfigRoot = defaultConfigRoot
	}
	if options.ArtifactConfigPath == "" {
		options.ArtifactConfigPath = "/etc/agentforge/artifacts/config.json"
	}
	if options.TerminationLogPath == "" {
		options.TerminationLogPath = "/dev/termination-log"
	}
	config, err := LoadRuntimeConfig(options.ConfigPath)
	if err != nil {
		return ExitConfig, err
	}
	task, err := LoadSignedTask(config, options.SecretRoot, options.ConfigRoot)
	if err != nil {
		return ExitConfig, err
	}
	if err := os.MkdirAll(options.Workspace, 0o750); err != nil {
		return ExitInternal, fmt.Errorf("create workspace: %w", err)
	}
	redactor := NewRedactor(options.SecretRoot, config.SecretRefs)
	var trajectory bytes.Buffer
	sink := NewTrajectorySink(io.MultiWriter(options.LogWriter, &trajectory), redactor)
	ctx, cancel := context.WithTimeout(parent, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	heartbeatsDone := make(chan struct{})
	defer close(heartbeatsDone)
	go emitHeartbeats(ctx, heartbeatsDone, sink)
	if err := sink.Emit("run.started", "deterministic runner started"); err != nil {
		return ExitInternal, err
	}
	if err := writePythonTemplate(options.Workspace, task.Template); err != nil {
		return ExitInternal, err
	}
	if err := sink.Emit("workspace.ready", "controlled template created"); err != nil {
		return ExitInternal, err
	}
	results, err := runTests(ctx, options.Workspace, task, sink)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ExitDeadline, err
		}
		if errors.Is(err, context.Canceled) {
			return ExitCancelled, err
		}
		if errors.Is(err, ErrCommandPolicy) {
			return ExitPolicy, err
		}
		return ExitCommand, err
	}
	if err := sink.Emit("tests.passed", "controlled template checks passed"); err != nil {
		return ExitInternal, err
	}
	store, err := LoadArtifactStore(options.ArtifactConfigPath, options.Workspace, options.SecretRoot, config.SecretRefs)
	if err != nil {
		return ExitArtifacts, err
	}
	reference, err := publishArtifacts(ctx, store, config, options.Workspace, trajectory.Bytes(), results)
	if err != nil {
		return ExitArtifacts, err
	}
	if err := writeTerminationEvidence(options.TerminationLogPath, reference); err != nil {
		return ExitArtifacts, err
	}
	_ = sink.Emit("run.succeeded", "mandatory artifacts are durable")
	return ExitSuccess, nil
}

func writeTerminationEvidence(path, reference string) error {
	contents, err := json.Marshal(struct {
		SchemaVersion       int    `json:"schemaVersion"`
		ArtifactManifestRef string `json:"artifactManifestRef"`
	}{SchemaVersion: 1, ArtifactManifestRef: reference})
	if err != nil {
		return err
	}
	if len(contents) > 4096 {
		return fmt.Errorf("termination evidence exceeds limit")
	}
	return os.WriteFile(path, contents, 0o640)
}

func emitHeartbeats(ctx context.Context, done <-chan struct{}, sink *TrajectorySink) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			_ = sink.Emit("heartbeat", "runner is active")
		}
	}
}

func writePythonTemplate(workspace, template string) error {
	if template != "python-basic" {
		return fmt.Errorf("unsupported controlled template")
	}
	if err := os.WriteFile(filepath.Join(workspace, "app.py"), []byte("def add(left, right):\n    return left + right\n"), 0o640); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workspace, "test_app.py"), []byte("import unittest\nfrom app import add\n\nclass AppTest(unittest.TestCase):\n    def test_add(self):\n        self.assertEqual(add(2, 3), 5)\n\nif __name__ == '__main__':\n    unittest.main()\n"), 0o640)
}

func runTests(ctx context.Context, workspace string, task TaskEnvelope, sink *TrajectorySink) ([]CommandResult, error) {
	for _, name := range []string{"app.py", "test_app.py"} {
		if _, err := os.Stat(filepath.Join(workspace, name)); err != nil {
			return nil, fmt.Errorf("controlled template check: %w", err)
		}
	}
	executor := DefaultExecutor(workspace)
	results := make([]CommandResult, 0, len(task.Tests))
	for _, command := range task.Tests {
		if err := sink.Emit("command.started", command[0]); err != nil {
			return nil, err
		}
		result, err := executor.Run(ctx, command, ".", map[string]string{"PYTHONDONTWRITEBYTECODE": "1"})
		results = append(results, result)
		if err != nil {
			return results, err
		}
		if err := sink.Emit("command.finished", "controlled command completed"); err != nil {
			return nil, err
		}
	}
	return results, nil
}
