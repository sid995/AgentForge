package domain

import (
	"fmt"
	"regexp"
	"slices"
	"time"
)

type ClusterStatus string

const (
	ClusterActive      ClusterStatus = "ACTIVE"
	ClusterUnavailable ClusterStatus = "UNAVAILABLE"
)

var clusterIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)
var clusterRegionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type ExecutionCluster struct {
	ID                         string
	Region                     string
	Status                     ClusterStatus
	SupportedRuntimes          []string
	SupportedExecutionProfiles []string
	Maintenance                bool
	TenantRestricted           bool
	SchedulingWeight           int
	CPUCostMinorUnits          int
	MemoryGiBCostMinorUnits    int
	LastHeartbeatAt            time.Time
	Version                    int64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type ClusterCapacity struct {
	ClusterID            string
	ObservedAt           time.Time
	AllocatableCPUMillis int64
	AllocatableMemoryMiB int64
	QueuedWorkloads      int
}

type ClusterCandidate struct {
	Cluster  ExecutionCluster
	Capacity ClusterCapacity
}

func NewExecutionCluster(cluster ExecutionCluster, now time.Time) (ExecutionCluster, error) {
	cluster.Version = 1
	cluster.CreatedAt = now.UTC()
	cluster.UpdatedAt = now.UTC()
	if err := cluster.Validate(now); err != nil {
		return ExecutionCluster{}, err
	}
	return cluster, nil
}

func (cluster ExecutionCluster) Validate(now time.Time) error {
	if !clusterIDPattern.MatchString(cluster.ID) || !clusterRegionPattern.MatchString(cluster.Region) || (cluster.Status != ClusterActive && cluster.Status != ClusterUnavailable) || cluster.SchedulingWeight < 1 || cluster.SchedulingWeight > 1000 || cluster.CPUCostMinorUnits < 0 || cluster.MemoryGiBCostMinorUnits < 0 || cluster.Version < 1 || cluster.LastHeartbeatAt.IsZero() || cluster.LastHeartbeatAt.After(now.UTC().Add(time.Minute)) || !validClusterValues(cluster.SupportedRuntimes, 64) || !validClusterValues(cluster.SupportedExecutionProfiles, 32) {
		return fmt.Errorf("execution cluster metadata is invalid")
	}
	return nil
}

func (capacity ClusterCapacity) Validate(now time.Time) error {
	if capacity.ClusterID == "" || capacity.ObservedAt.IsZero() || capacity.ObservedAt.After(now.UTC().Add(time.Minute)) || capacity.AllocatableCPUMillis < 0 || capacity.AllocatableMemoryMiB < 0 || capacity.QueuedWorkloads < 0 {
		return fmt.Errorf("cluster capacity metadata is invalid")
	}
	return nil
}

func validClusterValues(values []string, maximum int) bool {
	if len(values) == 0 || len(values) > maximum {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !schedulerPolicyValuePattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (cluster ExecutionCluster) Supports(runtime, profile string) bool {
	return slices.Contains(cluster.SupportedRuntimes, runtime) && slices.Contains(cluster.SupportedExecutionProfiles, profile)
}
