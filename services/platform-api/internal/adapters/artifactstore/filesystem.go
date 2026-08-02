// Package artifactstore contains durable object-storage adapters for Platform API services.
package artifactstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// FilesystemStore is a local conformance-test adapter. Its root is supplied by
// trusted server configuration and never by an artifact caller.
type FilesystemStore struct{ root string }

func NewFilesystemStore(root string) (*FilesystemStore, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("artifact filesystem root must be absolute")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create artifact filesystem root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact filesystem root: %w", err)
	}
	return &FilesystemStore{root: canonicalRoot}, nil
}

func (store *FilesystemStore) Put(_ context.Context, key ports.ArtifactKey, upload ports.ArtifactUpload) (ports.ArtifactObject, error) {
	path, err := store.path(key)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	if err := validateUpload(upload); err != nil {
		return ports.ArtifactObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return ports.ArtifactObject{}, fmt.Errorf("create artifact directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return ports.ArtifactObject{}, fmt.Errorf("create temporary artifact: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()

	written, digest, err := copyWithDigest(temporary, upload.Contents)
	if err != nil {
		_ = temporary.Close()
		return ports.ArtifactObject{}, err
	}
	if written != upload.Size || digest != upload.SHA256 {
		_ = temporary.Close()
		return ports.ArtifactObject{}, fmt.Errorf("artifact content does not match declared metadata")
	}
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return ports.ArtifactObject{}, fmt.Errorf("set artifact permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return ports.ArtifactObject{}, fmt.Errorf("close temporary artifact: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return ports.ArtifactObject{}, fmt.Errorf("commit artifact: %w", err)
	}
	return ports.ArtifactObject{Key: key, Size: written, SHA256: digest}, nil
}

func (store *FilesystemStore) Get(ctx context.Context, key ports.ArtifactKey) (ports.ArtifactObject, io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return ports.ArtifactObject{}, nil, err
	}
	object, err := store.Head(ctx, key)
	if err != nil {
		return ports.ArtifactObject{}, nil, err
	}
	path, err := store.path(key)
	if err != nil {
		return ports.ArtifactObject{}, nil, err
	}
	contents, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ports.ArtifactObject{}, nil, fmt.Errorf("%w: %s", ports.ErrArtifactNotFound, key.Name)
	}
	if err != nil {
		return ports.ArtifactObject{}, nil, fmt.Errorf("open artifact: %w", err)
	}
	return object, contents, nil
}

func (store *FilesystemStore) Head(ctx context.Context, key ports.ArtifactKey) (ports.ArtifactObject, error) {
	if err := ctx.Err(); err != nil {
		return ports.ArtifactObject{}, err
	}
	path, err := store.path(key)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	contents, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ports.ArtifactObject{}, fmt.Errorf("%w: %s", ports.ErrArtifactNotFound, key.Name)
	}
	if err != nil {
		return ports.ArtifactObject{}, fmt.Errorf("open artifact: %w", err)
	}
	defer func() { _ = contents.Close() }()
	size, digest, err := copyWithDigest(io.Discard, contents)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	return ports.ArtifactObject{Key: key, Size: size, SHA256: digest}, nil
}

func (store *FilesystemStore) Delete(ctx context.Context, key ports.ArtifactKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := store.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ports.ErrArtifactNotFound, key.Name)
	} else if err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}
	return nil
}

func (store *FilesystemStore) PresignDownload(_ context.Context, _ ports.ArtifactKey, lifetime time.Duration) (ports.PresignedDownload, error) {
	if err := validatePresignLifetime(lifetime); err != nil {
		return ports.PresignedDownload{}, err
	}
	return ports.PresignedDownload{}, ports.ErrPresigningUnsupported
}

func (store *FilesystemStore) path(key ports.ArtifactKey) (string, error) {
	name, err := key.ObjectName()
	if err != nil {
		return "", err
	}
	path := filepath.Join(store.root, filepath.FromSlash(name))
	relative, err := filepath.Rel(store.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path escapes filesystem root")
	}
	return path, nil
}

func validateUpload(upload ports.ArtifactUpload) error {
	if upload.Contents == nil || upload.Size < 0 || upload.ContentType == "" || len(upload.SHA256) != sha256.Size*2 {
		return fmt.Errorf("artifact upload metadata is invalid")
	}
	if _, err := hex.DecodeString(upload.SHA256); err != nil || strings.ToLower(upload.SHA256) != upload.SHA256 {
		return fmt.Errorf("artifact upload checksum is invalid")
	}
	return nil
}

func validatePresignLifetime(lifetime time.Duration) error {
	if lifetime <= 0 || lifetime > ports.MaxArtifactPresignLifetime {
		return fmt.Errorf("artifact presign lifetime is outside the approved range")
	}
	return nil
}

func copyWithDigest(destination io.Writer, source io.Reader) (int64, string, error) {
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, digest), source)
	if err != nil {
		return 0, "", fmt.Errorf("copy artifact: %w", err)
	}
	return written, hex.EncodeToString(digest.Sum(nil)), nil
}

var _ ports.ArtifactStore = (*FilesystemStore)(nil)
