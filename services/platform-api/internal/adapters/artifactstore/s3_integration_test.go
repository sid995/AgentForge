//go:build artifactintegration

package artifactstore

import (
	"context"
	"os"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestS3StoreConformance(t *testing.T) {
	endpoint := os.Getenv("AGENTFORGE_TEST_MINIO_ENDPOINT")
	accessKey := os.Getenv("AGENTFORGE_TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("AGENTFORGE_TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("MinIO test configuration is required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: false, TrailingHeaders: true})
	if err != nil {
		t.Fatal(err)
	}
	const bucket = "artifact-store-integration"
	if err := client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); err != nil {
		exists, existsErr := client.BucketExists(context.Background(), bucket)
		if existsErr != nil || !exists {
			t.Fatalf("create bucket: %v exists=%v existsErr=%v", err, exists, existsErr)
		}
	}
	store, err := NewS3Store(client, bucket, "phase-8")
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactStoreConformance(t, store, true)
}
