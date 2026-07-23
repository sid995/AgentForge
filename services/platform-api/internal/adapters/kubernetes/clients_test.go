package kubernetes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	handoffapp "github.com/sid995/agentforge/services/platform-api/internal/application/handoff"
)

func TestClientSelectorUsesOnlyExplicitClusterContextMapping(t *testing.T) {
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- name: cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: context-east
  context:
    cluster: cluster
    user: handoff
current-context: context-east
users:
- name: handoff
  user:
    token: test-only
`
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	selector, err := NewClientSelector(path, map[string]string{"cluster-east": "context-east"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := selector.Select(context.Background(), "cluster-east")
	if err != nil {
		t.Fatal(err)
	}
	second, err := selector.Select(context.Background(), "cluster-east")
	if err != nil || first != second {
		t.Fatalf("client was not cached: first=%p second=%p err=%v", first, second, err)
	}
	if _, err := selector.Select(context.Background(), "cluster-west"); !errors.Is(err, handoffapp.ErrUnauthorizedCluster) {
		t.Fatalf("unmapped cluster error=%v", err)
	}
	if _, err := NewClientSelector(path, map[string]string{"cluster-west": "missing"}); err == nil {
		t.Fatal("unknown kubeconfig context mapping accepted")
	}
	if _, err := NewClientSelector(path, map[string]string{"cluster-east": "context-east", "cluster-alias": "context-east"}); err == nil {
		t.Fatal("duplicate kubeconfig context mapping accepted")
	}
}
