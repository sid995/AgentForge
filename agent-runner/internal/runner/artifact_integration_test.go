//go:build runnerintegration

package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestS3CompatibleArtifactPublication(t *testing.T) {
	endpoint, accessKey, secretKey := os.Getenv("AGENTFORGE_TEST_MINIO_ENDPOINT"), os.Getenv("AGENTFORGE_TEST_MINIO_ACCESS_KEY"), os.Getenv("AGENTFORGE_TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("MinIO test configuration is required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bucket := "runner-integration"
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		exists, existsErr := client.BucketExists(ctx, bucket)
		if existsErr != nil || !exists {
			t.Fatalf("create bucket: %v exists=%v existsErr=%v", err, exists, existsErr)
		}
	}
	root := t.TempDir()
	secretRoot := filepath.Join(root, "secrets")
	if err := os.MkdirAll(filepath.Join(secretRoot, "artifact-credential"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretRoot, "artifact-credential", "accessKey"), []byte(accessKey), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretRoot, "artifact-credential", "secretKey"), []byte(secretKey), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "artifact-config.json")
	config := `{"schemaVersion":1,"backend":"s3","endpoint":"` + endpoint + `","bucket":"` + bucket + `","secure":false,"prefix":"phase-7","credentialRef":"artifact-credential"}`
	if err := os.WriteFile(configPath, []byte(config), 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := LoadArtifactStore(configPath, root, secretRoot)
	if err != nil {
		t.Fatal(err)
	}
	object, err := putVerified(ctx, store, "tenants/t/projects/p/runs/r/attempts/1/result.json", "application/json", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if object.SHA256 == "" || store.Reference(object.Key) == "" {
		t.Fatalf("invalid object: %+v", object)
	}
}
