package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const artifactSchemaVersion = 1

type ArtifactConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	Backend       string `json:"backend"`
	Root          string `json:"root,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	Bucket        string `json:"bucket,omitempty"`
	Region        string `json:"region,omitempty"`
	Secure        bool   `json:"secure,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
	CredentialRef string `json:"credentialRef,omitempty"`
}

type ArtifactObject struct {
	Key, ContentType, SHA256 string
	Size                     int64
}
type ArtifactStore interface {
	Put(context.Context, string, string, []byte) (ArtifactObject, error)
	Head(context.Context, string) (ArtifactObject, error)
	Reference(string) string
}
type filesystemStore struct{ root string }

func LoadArtifactStore(configPath, workspace, secretRoot string, approvedSecretRefs []string) (ArtifactStore, error) {
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read artifact configuration: %w", err)
	}
	decoder := json.NewDecoder(bytesReader(contents))
	decoder.DisallowUnknownFields()
	var config ArtifactConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode artifact configuration: %w", err)
	}
	if config.SchemaVersion != artifactSchemaVersion {
		return nil, fmt.Errorf("artifact configuration schema is unsupported")
	}
	switch config.Backend {
	case "filesystem":
		if config.Root == "" {
			return nil, fmt.Errorf("filesystem artifact root is required")
		}
		root, err := confinedArtifactRoot(workspace, config.Root)
		if err != nil {
			return nil, err
		}
		return filesystemStore{root: root}, nil
	case "s3":
		if config.Endpoint == "" || config.Bucket == "" || config.CredentialRef == "" || !referenceNamePattern.MatchString(config.CredentialRef) || !slices.Contains(approvedSecretRefs, config.CredentialRef) {
			return nil, fmt.Errorf("S3 artifact configuration is invalid")
		}
		accessKey, err := os.ReadFile(filepath.Join(secretRoot, config.CredentialRef, "accessKey"))
		if err != nil {
			return nil, fmt.Errorf("read artifact access key: %w", err)
		}
		secretKey, err := os.ReadFile(filepath.Join(secretRoot, config.CredentialRef, "secretKey"))
		if err != nil {
			return nil, fmt.Errorf("read artifact secret key: %w", err)
		}
		sessionToken, _ := os.ReadFile(filepath.Join(secretRoot, config.CredentialRef, "sessionToken"))
		client, err := minio.New(config.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(string(accessKey), string(secretKey), string(sessionToken)), Secure: config.Secure, Region: config.Region})
		if err != nil {
			return nil, fmt.Errorf("create S3 artifact client: %w", err)
		}
		return s3Store{client: client, bucket: config.Bucket, prefix: strings.Trim(config.Prefix, "/")}, nil
	default:
		return nil, fmt.Errorf("artifact backend is unsupported")
	}
}

func confinedArtifactRoot(workspace, root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("filesystem artifact root must be absolute")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", fmt.Errorf("create artifact root: %w", err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve artifact root: %w", err)
	}
	relative, err := filepath.Rel(canonicalWorkspace, canonicalRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("filesystem artifact root escapes workspace")
	}
	return canonicalRoot, nil
}

func (store filesystemStore) Put(_ context.Context, key, contentType string, contents []byte) (ArtifactObject, error) {
	path, err := filesystemPath(store.root, key)
	if err != nil {
		return ArtifactObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return ArtifactObject{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return ArtifactObject{}, err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return ArtifactObject{}, err
	}
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return ArtifactObject{}, err
	}
	if err := temporary.Close(); err != nil {
		return ArtifactObject{}, err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return ArtifactObject{}, err
	}
	return artifactObject(key, contentType, contents), nil
}
func (store filesystemStore) Head(_ context.Context, key string) (ArtifactObject, error) {
	path, err := filesystemPath(store.root, key)
	if err != nil {
		return ArtifactObject{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ArtifactObject{}, err
	}
	return artifactObject(key, "", contents), nil
}
func (store filesystemStore) Reference(key string) string {
	path, err := filesystemPath(store.root, key)
	if err != nil {
		return ""
	}
	return "file://" + path
}
func filesystemPath(root, key string) (string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", fmt.Errorf("artifact key is invalid")
	}
	path := filepath.Join(root, key)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("artifact key escapes root")
	}
	return path, nil
}

type s3Store struct {
	client         *minio.Client
	bucket, prefix string
}

func (store s3Store) fullKey(key string) string {
	if store.prefix == "" {
		return key
	}
	return store.prefix + "/" + key
}
func (store s3Store) Put(ctx context.Context, key, contentType string, contents []byte) (ArtifactObject, error) {
	object := artifactObject(key, contentType, contents)
	_, err := store.client.PutObject(ctx, store.bucket, store.fullKey(key), bytes.NewReader(contents), int64(len(contents)), minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"X-Amz-Meta-Sha256": object.SHA256}})
	if err != nil {
		return ArtifactObject{}, err
	}
	return object, nil
}
func (store s3Store) Head(ctx context.Context, key string) (ArtifactObject, error) {
	info, err := store.client.StatObject(ctx, store.bucket, store.fullKey(key), minio.StatObjectOptions{})
	if err != nil {
		return ArtifactObject{}, err
	}
	hash := ""
	for name, value := range info.UserMetadata {
		if strings.EqualFold(name, "X-Amz-Meta-Sha256") || strings.EqualFold(name, "sha256") {
			hash = value
			break
		}
	}
	return ArtifactObject{Key: key, ContentType: info.ContentType, SHA256: hash, Size: info.Size}, nil
}
func (store s3Store) Reference(key string) string {
	return "s3://" + store.bucket + "/" + store.fullKey(key)
}
func artifactObject(key, contentType string, contents []byte) ArtifactObject {
	digest := sha256.Sum256(contents)
	return ArtifactObject{Key: key, ContentType: contentType, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(contents))}
}

type ResultManifest struct {
	SchemaVersion   int              `json:"schemaVersion"`
	TenantID        string           `json:"tenantId"`
	ProjectID       string           `json:"projectId"`
	RunID           string           `json:"runId"`
	AttemptID       string           `json:"attemptId"`
	Attempt         int              `json:"attempt"`
	Outcome         string           `json:"outcome"`
	Artifacts       []ArtifactObject `json:"artifacts"`
	TrajectoryFirst uint64           `json:"trajectoryFirstSequence"`
	TrajectoryLast  uint64           `json:"trajectoryLastSequence"`
}

func publishArtifacts(ctx context.Context, store ArtifactStore, config RuntimeConfig, workspace string, trajectory []byte, results []CommandResult) (string, error) {
	prefix := strings.Join([]string{"tenants", config.TenantID, "projects", config.ProjectID, "runs", config.RunID, "attempts", fmt.Sprintf("%d", config.Attempt)}, "/")
	snapshot, err := sourceSnapshot(workspace)
	if err != nil {
		return "", err
	}
	objects := []struct {
		key, contentType string
		contents         []byte
	}{{prefix + "/source.tar.gz", "application/gzip", snapshot}, {prefix + "/test-report.json", "application/json", testReport(results)}, {prefix + "/trajectory/000001.jsonl", "application/x-ndjson", trajectory}, {prefix + "/logs/stdout.txt", "text/plain", commandOutput(results, true)}, {prefix + "/logs/stderr.txt", "text/plain", commandOutput(results, false)}}
	published := make([]ArtifactObject, 0, len(objects))
	for _, object := range objects {
		result, err := putVerified(ctx, store, object.key, object.contentType, object.contents)
		if err != nil {
			return "", err
		}
		published = append(published, result)
	}
	manifest := ResultManifest{SchemaVersion: 1, TenantID: config.TenantID, ProjectID: config.ProjectID, RunID: config.RunID, AttemptID: config.AttemptID, Attempt: config.Attempt, Outcome: "SUCCEEDED", Artifacts: published, TrajectoryFirst: 1, TrajectoryLast: trajectorySequence(trajectory)}
	contents, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	key := prefix + "/result-manifest.json"
	if _, err := putVerified(ctx, store, key, "application/json", contents); err != nil {
		return "", err
	}
	return store.Reference(key), nil
}

func putVerified(ctx context.Context, store ArtifactStore, key, contentType string, contents []byte) (ArtifactObject, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		result, err := store.Put(ctx, key, contentType, contents)
		if err == nil {
			head, headErr := store.Head(ctx, key)
			if headErr == nil && head.Size == result.Size && head.SHA256 == result.SHA256 {
				return result, nil
			}
			if headErr != nil {
				err = headErr
			} else {
				err = fmt.Errorf("artifact verification failed")
			}
		}
		last = err
		select {
		case <-ctx.Done():
			return ArtifactObject{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 25 * time.Millisecond):
		}
	}
	return ArtifactObject{}, fmt.Errorf("publish %s: %w", key, last)
}
func sourceSnapshot(workspace string) ([]byte, error) {
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	archive := tar.NewWriter(gzipWriter)
	if err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || strings.Contains(path, string(os.PathSeparator)+"artifacts"+string(os.PathSeparator)) {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source snapshot refuses symbolic links")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name, header.ModTime, header.AccessTime, header.ChangeTime = filepath.ToSlash(relative), time.Unix(0, 0), time.Time{}, time.Time{}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(archive, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func testReport(results []CommandResult) []byte {
	value, _ := json.Marshal(struct {
		SchemaVersion int             `json:"schemaVersion"`
		Passed        bool            `json:"passed"`
		Commands      []CommandResult `json:"commands"`
	}{SchemaVersion: 1, Passed: true, Commands: results})
	return value
}
func commandOutput(results []CommandResult, stdout bool) []byte {
	var output strings.Builder
	for _, result := range results {
		if stdout {
			output.WriteString(result.Stdout)
		} else {
			output.WriteString(result.Stderr)
		}
	}
	return []byte(output.String())
}
func trajectorySequence(contents []byte) uint64 {
	var last uint64
	for _, line := range bytes.Split(contents, []byte("\n")) {
		var record TrajectoryRecord
		if json.Unmarshal(line, &record) == nil {
			last = record.Sequence
		}
	}
	return last
}
