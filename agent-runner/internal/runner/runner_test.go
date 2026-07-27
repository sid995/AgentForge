package runner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLoadsSignedTaskAndRedactsLogs(t *testing.T) {
	temp := t.TempDir()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	configPath := writeFixture(t, temp, public, private, "secret-value")
	var logs strings.Builder
	code, err := Run(context.Background(), Options{ConfigPath: configPath, Workspace: filepath.Join(temp, "workspace"), SecretRoot: filepath.Join(temp, "secrets"), ConfigRoot: filepath.Join(temp, "config"), LogWriter: &logs})
	if code != ExitArtifacts || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(logs.String(), "workspace.ready") || strings.Contains(logs.String(), "secret-value") {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
	if _, err := os.Stat(filepath.Join(temp, "workspace", "app.py")); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsSignatureMismatch(t *testing.T) {
	temp := t.TempDir()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	configPath := writeFixture(t, temp, public, private, "value")
	if err := os.WriteFile(filepath.Join(temp, "secrets", "task", "envelope.json.sig"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, err := Run(context.Background(), Options{ConfigPath: configPath, Workspace: filepath.Join(temp, "workspace"), SecretRoot: filepath.Join(temp, "secrets"), ConfigRoot: filepath.Join(temp, "config")})
	if code != ExitConfig || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func writeFixture(t *testing.T, root string, public ed25519.PublicKey, private ed25519.PrivateKey, secret string) string {
	t.Helper()
	for _, path := range []string{filepath.Join(root, "secrets", "task"), filepath.Join(root, "secrets", "runner-secret"), filepath.Join(root, "config", "trust")} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	body := []byte(`{"schemaVersion":1,"keyId":"test","tenantId":"tenant","projectId":"project","runId":"run","attemptId":"attempt","attempt":1,"template":"python-basic","tests":[["/usr/local/bin/python3","-m","unittest"]]}`)
	if err := os.WriteFile(filepath.Join(root, "secrets", "task", "envelope.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secrets", "task", "envelope.json.sig"), []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, body))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secrets", "runner-secret", "token"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "trust", "task-trust.json"), []byte(fmt.Sprintf(`{"keys":{"test":"%s"}}`, base64.StdEncoding.EncodeToString(public))), 0o640); err != nil {
		t.Fatal(err)
	}
	config := `{"schemaVersion":1,"tenantId":"tenant","projectId":"project","runId":"run","attemptId":"attempt","attempt":1,"taskRef":"secret://task/envelope.json","timeoutSeconds":30,"configurationRefs":["trust"],"secretRefs":["runner-secret","task"],"artifactDestinationRef":"artifacts"}`
	path := filepath.Join(root, "runtime.json")
	if err := os.WriteFile(path, []byte(config), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}
