package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemArtifactsAreConfinedAndVerified(t *testing.T) {
	workspace := t.TempDir()
	store, err := LoadArtifactStore(writeArtifactConfig(t, workspace, filepath.Join(workspace, "artifacts")), workspace, filepath.Join(workspace, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	object, err := putVerified(context.Background(), store, "tenants/a/result.json", "application/json", []byte("result"))
	if err != nil {
		t.Fatal(err)
	}
	if object.SHA256 == "" || store.Reference(object.Key) == "" {
		t.Fatalf("object=%+v", object)
	}
	if _, err := filesystemPath(filepath.Join(workspace, "artifacts"), "../../escape"); err == nil {
		t.Fatal("traversal key accepted")
	}
	if _, err := confinedArtifactRoot(workspace, t.TempDir()); err == nil {
		t.Fatal("external artifact root accepted")
	}
}

func TestPutVerifiedRetriesAndRefusesChecksumMismatch(t *testing.T) {
	store := &memoryStore{failures: 1}
	if _, err := putVerified(context.Background(), store, "key", "text/plain", []byte("value")); err != nil || store.puts != 2 {
		t.Fatalf("err=%v puts=%d", err, store.puts)
	}
	store = &memoryStore{badHead: true}
	if _, err := putVerified(context.Background(), store, "key", "text/plain", []byte("value")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

type memoryStore struct {
	failures, puts int
	badHead        bool
	object         ArtifactObject
}

func (store *memoryStore) Put(_ context.Context, key, contentType string, contents []byte) (ArtifactObject, error) {
	store.puts++
	if store.failures > 0 {
		store.failures--
		return ArtifactObject{}, errors.New("temporary")
	}
	store.object = artifactObject(key, contentType, contents)
	return store.object, nil
}
func (store *memoryStore) Head(_ context.Context, _ string) (ArtifactObject, error) {
	object := store.object
	if store.badHead {
		object.SHA256 = "bad"
	}
	return object, nil
}
func (store *memoryStore) Reference(key string) string { return "memory://" + key }

func writeArtifactConfig(t *testing.T, workspace, root string) string {
	t.Helper()
	path := filepath.Join(workspace, "artifact-config.json")
	contents := []byte(`{"schemaVersion":1,"backend":"filesystem","root":"` + root + `"}`)
	if err := os.WriteFile(path, contents, 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}
