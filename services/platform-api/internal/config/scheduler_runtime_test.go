package config

import "testing"

func TestSchedulerConfigBoundsWorkersAndLease(t *testing.T) {
	values := map[string]string{"AGENTFORGE_DATABASE_URL": "postgres://user:pass@localhost/db?sslmode=disable", "AGENTFORGE_SCHEDULER_WORKERS": "8", "AGENTFORGE_SCHEDULER_WORK_QUEUE_SIZE": "16", "AGENTFORGE_SCHEDULER_STRATEGY": "region-affinity", "AGENTFORGE_SCHEDULER_RUNNER_IMAGES": `{"python-3.12":"registry.example.test/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, "AGENTFORGE_SCHEDULER_WORKSPACE_SIZE_GIB": "10", "AGENTFORGE_SCHEDULER_ARTIFACT_DESTINATION_REF": "artifact-store"}
	configuration, err := LoadScheduler(func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil || configuration.WorkerCount != 8 || configuration.Strategy != "region-affinity" {
		t.Fatalf("configuration=%#v err=%v", configuration, err)
	}
	values["AGENTFORGE_SCHEDULER_LEASE_DURATION"] = "1s"
	values["AGENTFORGE_SCHEDULER_POLL_INTERVAL"] = "2s"
	if _, err := LoadScheduler(func(name string) (string, bool) { value, ok := values[name]; return value, ok }); err == nil {
		t.Fatal("lease shorter than poll accepted")
	}
}
