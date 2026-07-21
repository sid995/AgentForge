package buildinfo

import "testing"

func TestCurrentReturnsBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})

	Version = "v1.2.3"
	Commit = "abc123"
	BuildTime = "2026-07-21T12:00:00Z"

	got := Current()
	if got.Version != Version || got.Commit != Commit || got.BuildTime != BuildTime {
		t.Fatalf("Current() = %#v, want linked metadata", got)
	}
}
