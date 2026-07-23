package config

import "testing"

func TestLoadHandoffRequiresExplicitClusterContexts(t *testing.T) {
	values := map[string]string{
		"AGENTFORGE_DATABASE_URL":                 "postgres://user:pass@localhost/db?sslmode=disable",
		"AGENTFORGE_HANDOFF_KUBECONFIG":           "/tmp/kubeconfig",
		"AGENTFORGE_HANDOFF_CLUSTER_CONTEXTS":     `{"cluster-east":"kind-agentforge"}`,
		"AGENTFORGE_HANDOFF_QUARANTINE_TENANT_ID": "019b0000-0000-7000-8000-000000000001",
	}
	configuration, err := LoadHandoff(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil || configuration.ClusterContexts["cluster-east"] != "kind-agentforge" {
		t.Fatalf("configuration=%#v err=%v", configuration, err)
	}
	values["AGENTFORGE_HANDOFF_CLUSTER_CONTEXTS"] = `{"INVALID":"context"}`
	if _, err := LoadHandoff(func(name string) (string, bool) { value, ok := values[name]; return value, ok }); err == nil {
		t.Fatal("invalid cluster context mapping accepted")
	}
}
