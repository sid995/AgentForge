package artifactstore

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

// S3Store stores objects in a server-configured S3-compatible bucket. The
// client and bucket are injected by trusted application configuration.
type S3Store struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3Store(client *minio.Client, bucket, prefix string) (*S3Store, error) {
	if client == nil || bucket == "" {
		return nil, fmt.Errorf("S3 artifact store configuration is invalid")
	}
	normalizedPrefix, err := normalizePrefix(prefix)
	if err != nil {
		return nil, err
	}
	return &S3Store{client: client, bucket: bucket, prefix: normalizedPrefix}, nil
}

func (store *S3Store) Put(ctx context.Context, key ports.ArtifactKey, upload ports.ArtifactUpload) (ports.ArtifactObject, error) {
	if err := validateUpload(upload); err != nil {
		return ports.ArtifactObject{}, err
	}
	name, err := store.name(key)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	_, err = store.client.PutObject(ctx, store.bucket, name, upload.Contents, upload.Size, minio.PutObjectOptions{
		ContentType: upload.ContentType,
		Checksum:    minio.ChecksumSHA256,
	})
	if err != nil {
		return ports.ArtifactObject{}, translateS3Error(err, key)
	}
	object, err := store.Head(ctx, key)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	if object.Size != upload.Size || object.SHA256 != upload.SHA256 {
		return ports.ArtifactObject{}, fmt.Errorf("S3 artifact does not match declared metadata")
	}
	return object, nil
}

func (store *S3Store) Get(ctx context.Context, key ports.ArtifactKey) (ports.ArtifactObject, io.ReadCloser, error) {
	object, err := store.Head(ctx, key)
	if err != nil {
		return ports.ArtifactObject{}, nil, err
	}
	name, err := store.name(key)
	if err != nil {
		return ports.ArtifactObject{}, nil, err
	}
	contents, err := store.client.GetObject(ctx, store.bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return ports.ArtifactObject{}, nil, translateS3Error(err, key)
	}
	return object, contents, nil
}

func (store *S3Store) Head(ctx context.Context, key ports.ArtifactKey) (ports.ArtifactObject, error) {
	name, err := store.name(key)
	if err != nil {
		return ports.ArtifactObject{}, err
	}
	info, err := store.client.StatObject(ctx, store.bucket, name, minio.StatObjectOptions{Checksum: true})
	if err != nil {
		return ports.ArtifactObject{}, translateS3Error(err, key)
	}
	digest, err := base64.StdEncoding.DecodeString(info.ChecksumSHA256)
	if err != nil || len(digest) != 32 {
		return ports.ArtifactObject{}, fmt.Errorf("S3 artifact checksum is invalid")
	}
	return ports.ArtifactObject{Key: key, Size: info.Size, SHA256: hex.EncodeToString(digest)}, nil
}

func (store *S3Store) Delete(ctx context.Context, key ports.ArtifactKey) error {
	name, err := store.name(key)
	if err != nil {
		return err
	}
	if err := store.client.RemoveObject(ctx, store.bucket, name, minio.RemoveObjectOptions{}); err != nil {
		return translateS3Error(err, key)
	}
	return nil
}

func (store *S3Store) PresignDownload(ctx context.Context, key ports.ArtifactKey, lifetime time.Duration) (ports.PresignedDownload, error) {
	if err := validatePresignLifetime(lifetime); err != nil {
		return ports.PresignedDownload{}, err
	}
	name, err := store.name(key)
	if err != nil {
		return ports.PresignedDownload{}, err
	}
	url, err := store.client.PresignedGetObject(ctx, store.bucket, name, lifetime, nil)
	if err != nil {
		return ports.PresignedDownload{}, translateS3Error(err, key)
	}
	return ports.PresignedDownload{URL: url, ExpiresAt: time.Now().UTC().Add(lifetime)}, nil
}

func (store *S3Store) name(key ports.ArtifactKey) (string, error) {
	objectName, err := key.ObjectName()
	if err != nil {
		return "", err
	}
	if store.prefix == "" {
		return objectName, nil
	}
	return store.prefix + "/" + objectName, nil
}

func normalizePrefix(prefix string) (string, error) {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return "", nil
	}
	if strings.Contains(prefix, "\\") || path.Clean(prefix) != prefix {
		return "", fmt.Errorf("S3 artifact prefix is invalid")
	}
	for _, component := range strings.Split(prefix, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("S3 artifact prefix is invalid")
		}
	}
	return prefix, nil
}

func translateS3Error(err error, key ports.ArtifactKey) error {
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NoSuchBucket" {
		return fmt.Errorf("%w: %s", ports.ErrArtifactNotFound, key.Name)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("S3 artifact operation failed: %w", err)
}

var _ ports.ArtifactStore = (*S3Store)(nil)
