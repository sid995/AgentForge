package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ArtifactRetentionClass string

const (
	ArtifactRetentionHot     ArtifactRetentionClass = "HOT"
	ArtifactRetentionArchive ArtifactRetentionClass = "ARCHIVE"
)

var artifactSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Artifact is immutable tenant-owned metadata for one durable object.
type Artifact struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	RunID          uuid.UUID
	AttemptID      uuid.UUID
	AttemptNumber  int
	ObjectKey      string
	ContentType    string
	SizeBytes      int64
	SHA256         string
	RetentionClass ArtifactRetentionClass
	CreatedBy      string
	CreatedAt      time.Time
}

type NewArtifactInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	RunID          uuid.UUID
	AttemptID      uuid.UUID
	AttemptNumber  int
	ObjectKey      string
	ContentType    string
	SizeBytes      int64
	SHA256         string
	RetentionClass ArtifactRetentionClass
	CreatedBy      string
}

// NewArtifact validates immutable storage metadata and assigns a UUIDv7.
func NewArtifact(input NewArtifactInput, now time.Time) (Artifact, error) {
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.SHA256 = strings.TrimSpace(input.SHA256)
	input.RetentionClass = ArtifactRetentionClass(strings.ToUpper(strings.TrimSpace(string(input.RetentionClass))))
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.TenantID == uuid.Nil || input.ProjectID == uuid.Nil || input.RunID == uuid.Nil || input.AttemptID == uuid.Nil || input.AttemptNumber < 1 || input.AttemptNumber > 10 {
		return Artifact{}, fmt.Errorf("%w: artifact ownership is invalid", ErrValidation)
	}
	prefix := fmt.Sprintf("tenants/%s/projects/%s/runs/%s/attempts/%d/", input.TenantID, input.ProjectID, input.RunID, input.AttemptNumber)
	if len(input.ObjectKey) <= len(prefix) || len(input.ObjectKey) > 2048 || !strings.HasPrefix(input.ObjectKey, prefix) || strings.Contains(input.ObjectKey, "\\") || hasUnsafeObjectKeySegment(input.ObjectKey) {
		return Artifact{}, fmt.Errorf("%w: artifact object key is invalid", ErrValidation)
	}
	if input.ContentType == "" || len(input.ContentType) > 255 || input.SizeBytes < 0 || !artifactSHA256Pattern.MatchString(input.SHA256) {
		return Artifact{}, fmt.Errorf("%w: artifact content metadata is invalid", ErrValidation)
	}
	if input.RetentionClass != ArtifactRetentionHot && input.RetentionClass != ArtifactRetentionArchive {
		return Artifact{}, fmt.Errorf("%w: artifact retention class is invalid", ErrValidation)
	}
	if input.CreatedBy == "" || len(input.CreatedBy) > 255 {
		return Artifact{}, fmt.Errorf("%w: artifact actor is invalid", ErrValidation)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Artifact{}, fmt.Errorf("generate artifact ID: %w", err)
	}
	return Artifact{ID: id, TenantID: input.TenantID, ProjectID: input.ProjectID, RunID: input.RunID, AttemptID: input.AttemptID, AttemptNumber: input.AttemptNumber, ObjectKey: input.ObjectKey, ContentType: input.ContentType, SizeBytes: input.SizeBytes, SHA256: input.SHA256, RetentionClass: input.RetentionClass, CreatedBy: input.CreatedBy, CreatedAt: now.UTC()}, nil
}

func hasUnsafeObjectKeySegment(key string) bool {
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return true
		}
	}
	return false
}
