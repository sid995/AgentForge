package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewArtifactValidatesTrustedObjectScope(t *testing.T) {
	input := validArtifactInput()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	artifact, err := NewArtifact(input, now)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ID.Version() != 7 || !artifact.CreatedAt.Equal(now) || artifact.RetentionClass != ArtifactRetentionHot {
		t.Fatalf("artifact = %#v", artifact)
	}
	input.ObjectKey = "tenants/other/projects/other/runs/other/attempts/1/logs/stdout.txt"
	if _, err := NewArtifact(input, now); !errors.Is(err, ErrValidation) {
		t.Fatalf("foreign object key error = %v, want validation", err)
	}
	input = validArtifactInput()
	input.ObjectKey += "/.."
	if _, err := NewArtifact(input, now); !errors.Is(err, ErrValidation) {
		t.Fatalf("traversal object key error = %v, want validation", err)
	}
	input = validArtifactInput()
	input.SHA256 = "invalid"
	if _, err := NewArtifact(input, now); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid checksum error = %v, want validation", err)
	}
}

func validArtifactInput() NewArtifactInput {
	tenantID, projectID, runID, attemptID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	return NewArtifactInput{TenantID: tenantID, ProjectID: projectID, RunID: runID, AttemptID: attemptID, AttemptNumber: 1, ObjectKey: fmt.Sprintf("tenants/%s/projects/%s/runs/%s/attempts/1/logs/stdout.txt", tenantID, projectID, runID), ContentType: "text/plain", SizeBytes: 42, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", RetentionClass: ArtifactRetentionHot, CreatedBy: "developer@example.test"}
}
