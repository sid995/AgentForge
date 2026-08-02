package ports

import (
	"testing"

	"github.com/google/uuid"
)

func TestArtifactKeyObjectName(t *testing.T) {
	key := ArtifactKey{
		TenantID:  uuid.MustParse("019b0000-0000-7000-8000-000000000001"),
		ProjectID: uuid.MustParse("019b0000-0000-7000-8000-000000000002"),
		RunID:     uuid.MustParse("019b0000-0000-7000-8000-000000000003"),
		Attempt:   2,
		Name:      "logs/stdout.txt",
	}
	got, err := key.ObjectName()
	if err != nil {
		t.Fatal(err)
	}
	want := "tenants/019b0000-0000-7000-8000-000000000001/projects/019b0000-0000-7000-8000-000000000002/runs/019b0000-0000-7000-8000-000000000003/attempts/2/logs/stdout.txt"
	if got != want {
		t.Fatalf("object key = %q, want %q", got, want)
	}
	key.TenantID = uuid.MustParse("019b0000-0000-7000-8000-000000000004")
	otherTenant, err := key.ObjectName()
	if err != nil {
		t.Fatal(err)
	}
	if otherTenant == got {
		t.Fatal("different tenants produced the same object key")
	}
}

func TestArtifactKeyRejectsUnsafeName(t *testing.T) {
	key := ArtifactKey{TenantID: uuid.New(), ProjectID: uuid.New(), RunID: uuid.New(), Attempt: 1}
	for _, name := range []string{"", "../escape", "logs/../escape", "/absolute", "logs\\stderr.txt"} {
		t.Run(name, func(t *testing.T) {
			key.Name = name
			if _, err := key.ObjectName(); err == nil {
				t.Fatal("unsafe artifact name was accepted")
			}
		})
	}
}
