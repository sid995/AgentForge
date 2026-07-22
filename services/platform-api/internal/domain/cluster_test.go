package domain

import (
	"testing"
	"time"
)

func TestExecutionClusterRejectsUntrustedMetadata(t *testing.T) {
	now := time.Now().UTC()
	valid := ExecutionCluster{ID: "cluster-one", Region: "us-east-1", Status: ClusterActive, SupportedRuntimes: []string{"python-3.12"}, SupportedExecutionProfiles: []string{"standard"}, SchedulingWeight: 100, LastHeartbeatAt: now}
	cluster, err := NewExecutionCluster(valid, now)
	if err != nil || !cluster.Supports("python-3.12", "standard") {
		t.Fatalf("cluster=%#v err=%v", cluster, err)
	}
	invalid := valid
	invalid.ID = "../../unsafe"
	if _, err := NewExecutionCluster(invalid, now); err == nil {
		t.Fatal("unsafe cluster identity accepted")
	}
	invalid = valid
	invalid.LastHeartbeatAt = now.Add(2 * time.Minute)
	if _, err := NewExecutionCluster(invalid, now); err == nil {
		t.Fatal("future-skewed heartbeat accepted")
	}
}
