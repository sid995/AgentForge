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

func TestNewRedactorRejectsEscapingProjectionReferences(t *testing.T) {
	secretRoot := t.TempDir()
	for _, reference := range []string{".", "../outside", filepath.Join("task", "..", "outside"), filepath.Join(secretRoot, "task")} {
		t.Run(reference, func(t *testing.T) {
			if _, err := NewRedactor(secretRoot, []string{reference}); err == nil {
				t.Fatal("expected invalid secret projection reference to fail")
			}
		})
	}
}

func TestNewRedactorRejectsSecretFileSymlinkOutsideProjection(t *testing.T) {
	secretRoot := t.TempDir()
	projection := filepath.Join(secretRoot, "task")
	if err := os.MkdirAll(projection, 0o750); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(external, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(projection, "token")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRedactor(secretRoot, []string{"task"}); err == nil {
		t.Fatal("expected outbound secret-file symlink to fail")
	}
}

func TestNewRedactorRejectsSymlinkProjection(t *testing.T) {
	secretRoot := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(secretRoot, "task")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRedactor(secretRoot, []string{"task"}); err == nil {
		t.Fatal("expected symlink secret projection to fail")
	}
}
