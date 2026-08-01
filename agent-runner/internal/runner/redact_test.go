package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRedactorSkipsKubernetesSecretProjectionDirectory(t *testing.T) {
	secretRoot := t.TempDir()
	reference := filepath.Join(secretRoot, "task")
	version := filepath.Join(reference, "..2026_08_02_000000")
	if err := os.MkdirAll(version, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(version, "token"), []byte("secret-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(version), filepath.Join(reference, "..data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..data", "token"), filepath.Join(reference, "token")); err != nil {
		t.Fatal(err)
	}

	redactor, err := NewRedactor(secretRoot, []string{"task"})
	if err != nil {
		t.Fatal(err)
	}
	if got := redactor.Redact("token=secret-value"); got != "token=[REDACTED]" {
		t.Fatalf("redacted=%q", got)
	}
}

func TestNewRedactorRejectsUnreadableSecretProjectionEntry(t *testing.T) {
	secretRoot := t.TempDir()
	reference := filepath.Join(secretRoot, "task")
	if err := os.MkdirAll(reference, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(reference, "token")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRedactor(secretRoot, []string{"task"}); err == nil {
		t.Fatal("expected unreadable secret projection entry to fail")
	}
}
