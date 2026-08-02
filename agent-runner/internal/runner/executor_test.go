package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorRejectsTraversalSymlinkShellAndEnvironment(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	executor := Executor{Workspace: workspace, AllowedExecutables: []string{os.Args[0]}}
	for _, test := range []struct {
		name, directory string
		arguments       []string
		environment     map[string]string
	}{
		{"traversal", "../", []string{os.Args[0]}, nil},
		{"symlink", "escape", []string{os.Args[0]}, nil},
		{"shell", ".", []string{"/bin/sh", "-c", "true"}, nil},
		{"environment", ".", []string{os.Args[0]}, map[string]string{"LD_PRELOAD": "bad"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := executor.Run(context.Background(), test.arguments, test.directory, test.environment)
			if !errors.Is(err, ErrCommandPolicy) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestExecutorTerminatesHungProcessGroup(t *testing.T) {
	workspace := t.TempDir()
	executor := Executor{Workspace: workspace, AllowedExecutables: []string{os.Args[0]}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := executor.Run(ctx, []string{os.Args[0], "-test.run=TestExecutorHelperProcess", "--", "sleep"}, ".", nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 3*time.Second {
		t.Fatalf("err=%v elapsed=%s", err, time.Since(started))
	}
}

func TestExecutorBoundsOutput(t *testing.T) {
	workspace := t.TempDir()
	executor := Executor{Workspace: workspace, AllowedExecutables: []string{os.Args[0]}, OutputLimit: 32}
	_, err := executor.Run(context.Background(), []string{os.Args[0], "-test.run=TestExecutorHelperProcess", "--", "spam"}, ".", nil)
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("err=%v", err)
	}
}

func TestExecutorHelperProcess(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "sleep":
		time.Sleep(30 * time.Second)
	case "spam":
		for index := 0; index < 1024; index++ {
			_, _ = os.Stdout.WriteString("0123456789")
		}
	}
	os.Exit(0)
}
