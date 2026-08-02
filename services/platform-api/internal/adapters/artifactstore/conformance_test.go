package artifactstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

func TestFilesystemStoreConformance(t *testing.T) {
	store, err := NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactStoreConformance(t, store, false)
}

func TestFilesystemStoreRejectsMismatchedChecksum(t *testing.T) {
	store, err := NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := ports.ArtifactKey{TenantID: uuid.New(), ProjectID: uuid.New(), RunID: uuid.New(), Attempt: 1, Name: "logs/stderr.txt"}
	_, err = store.Put(context.Background(), key, ports.ArtifactUpload{Contents: bytes.NewReader([]byte("contents")), ContentType: "text/plain", Size: int64(len("contents")), SHA256: strings.Repeat("0", sha256.Size*2)})
	if err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	if _, err := store.Head(context.Background(), key); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Fatalf("mismatched artifact was persisted: %v", err)
	}
}

func assertArtifactStoreConformance(t *testing.T, store ports.ArtifactStore, supportsPresigning bool) {
	t.Helper()
	ctx := context.Background()
	key := ports.ArtifactKey{TenantID: uuid.New(), ProjectID: uuid.New(), RunID: uuid.New(), Attempt: 1, Name: "logs/stdout.txt"}
	contents := []byte("deterministic artifact contents")
	digest := sha256.Sum256(contents)
	upload := ports.ArtifactUpload{Contents: bytes.NewReader(contents), ContentType: "text/plain", Size: int64(len(contents)), SHA256: hex.EncodeToString(digest[:])}

	put, err := store.Put(ctx, key, upload)
	if err != nil {
		t.Fatal(err)
	}
	if put.Size != int64(len(contents)) || put.SHA256 != upload.SHA256 {
		t.Fatalf("put object = %+v", put)
	}
	head, err := store.Head(ctx, key)
	if err != nil || head.Size != put.Size || head.SHA256 != put.SHA256 {
		t.Fatalf("head object = %+v, err = %v", head, err)
	}
	object, reader, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	got, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, contents) || object.SHA256 != put.SHA256 {
		t.Fatalf("get contents = %q, object = %+v, err = %v", got, object, err)
	}

	presigned, err := store.PresignDownload(ctx, key, time.Minute)
	if supportsPresigning {
		if err != nil || presigned.URL == nil || presigned.ExpiresAt.Before(time.Now()) {
			t.Fatalf("presigned download = %+v, err = %v", presigned, err)
		}
	} else if !errors.Is(err, ports.ErrPresigningUnsupported) {
		t.Fatalf("presigning error = %v, want unsupported", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Head(ctx, key); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Fatalf("head after delete error = %v, want not found", err)
	}
}
