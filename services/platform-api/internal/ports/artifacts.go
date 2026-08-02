package ports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxArtifactPresignLifetime = 15 * time.Minute

var (
	ErrArtifactNotFound      = errors.New("artifact not found")
	ErrPresigningUnsupported = errors.New("artifact presigning is unsupported")
)

// ArtifactKey identifies an object with the trusted execution identity that
// owns it. Callers cannot supply a free-form object-storage prefix.
type ArtifactKey struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	RunID     uuid.UUID
	Attempt   int
	Name      string
}

// ObjectName returns the canonical, collision-resistant object-storage key.
func (key ArtifactKey) ObjectName() (string, error) {
	if key.TenantID == uuid.Nil || key.ProjectID == uuid.Nil || key.RunID == uuid.Nil || key.Attempt < 1 {
		return "", fmt.Errorf("artifact scope is invalid")
	}
	if key.Name == "" || strings.Contains(key.Name, "\\") || path.IsAbs(key.Name) || path.Clean(key.Name) != key.Name {
		return "", fmt.Errorf("artifact name is invalid")
	}
	for _, component := range strings.Split(key.Name, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("artifact name is invalid")
		}
	}
	return fmt.Sprintf("tenants/%s/projects/%s/runs/%s/attempts/%d/%s", key.TenantID, key.ProjectID, key.RunID, key.Attempt, key.Name), nil
}

// ArtifactUpload is a stream with immutable content metadata supplied by a
// trusted producer. Adapters verify both size and SHA-256 before success.
type ArtifactUpload struct {
	Contents    io.Reader
	ContentType string
	Size        int64
	SHA256      string
}

// ArtifactObject is the server-observed representation of one durable object.
type ArtifactObject struct {
	Key    ArtifactKey
	Size   int64
	SHA256 string
}

// PresignedDownload is a short-lived server-generated read capability.
type PresignedDownload struct {
	URL       *url.URL
	ExpiresAt time.Time
}

// ArtifactStore owns durable payload bytes. PostgreSQL metadata and API
// authorization remain separate Phase 8 responsibilities.
type ArtifactStore interface {
	Put(context.Context, ArtifactKey, ArtifactUpload) (ArtifactObject, error)
	Get(context.Context, ArtifactKey) (ArtifactObject, io.ReadCloser, error)
	Head(context.Context, ArtifactKey) (ArtifactObject, error)
	Delete(context.Context, ArtifactKey) error
	PresignDownload(context.Context, ArtifactKey, time.Duration) (PresignedDownload, error)
}
