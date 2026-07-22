// Command clusterctl manages trusted execution-cluster registry records.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.LookupEnv); err != nil {
		fmt.Fprintln(os.Stderr, "clusterctl:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, lookup config.LookupEnv) error {
	if len(arguments) == 0 || (arguments[0] != "register" && arguments[0] != "update") {
		return fmt.Errorf("usage: clusterctl <register|update> [flags]")
	}
	command := arguments[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	id := flags.String("id", "", "stable cluster ID")
	region := flags.String("region", "", "cluster region")
	status := flags.String("status", string(domain.ClusterActive), "ACTIVE or UNAVAILABLE")
	runtimes := flags.String("runtimes", "", "comma-separated supported runtimes")
	profiles := flags.String("profiles", "", "comma-separated execution profiles")
	maintenance := flags.Bool("maintenance", false, "exclude cluster for maintenance")
	restricted := flags.Bool("tenant-restricted", false, "restrict cluster to allowlisted tenants")
	allowedTenants := flags.String("allowed-tenants", "", "comma-separated tenant UUIDs")
	weight := flags.Int("weight", 100, "scheduling weight")
	cpuCost := flags.Int("cpu-cost", 0, "CPU cost in minor units")
	memoryCost := flags.Int("memory-cost", 0, "GiB memory cost in minor units")
	cpu := flags.Int64("cpu-millis", 0, "allocatable CPU millicores")
	memory := flags.Int64("memory-mib", 0, "allocatable memory MiB")
	queued := flags.Int("queued-workloads", 0, "reported queued workloads")
	expectedVersion := flags.Int64("expected-version", 0, "required current version for update")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	configuration, err := config.Load(lookup)
	if err != nil {
		return err
	}
	pool, err := database.Open(ctx, configuration.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	repository := postgresadapter.NewClusterRegistry(pool)
	now := time.Now().UTC()
	cluster := domain.ExecutionCluster{
		ID: *id, Region: *region, Status: domain.ClusterStatus(*status),
		SupportedRuntimes: splitValues(*runtimes), SupportedExecutionProfiles: splitValues(*profiles),
		Maintenance: *maintenance, TenantRestricted: *restricted, SchedulingWeight: *weight,
		CPUCostMinorUnits: *cpuCost, MemoryGiBCostMinorUnits: *memoryCost, LastHeartbeatAt: now,
	}
	capacity := domain.ClusterCapacity{ClusterID: *id, ObservedAt: now, AllocatableCPUMillis: *cpu, AllocatableMemoryMiB: *memory, QueuedWorkloads: *queued}
	tenantIDs, err := parseTenantIDs(*allowedTenants)
	if err != nil {
		return err
	}
	if command == "register" {
		cluster, err = domain.NewExecutionCluster(cluster, now)
		if err == nil {
			err = repository.Register(ctx, cluster, capacity, tenantIDs)
		}
	} else {
		if *expectedVersion < 1 {
			return fmt.Errorf("-expected-version must be positive for update")
		}
		current, _, getErr := repository.Get(ctx, *id)
		if getErr != nil {
			return getErr
		}
		cluster.CreatedAt = current.CreatedAt
		cluster.UpdatedAt = now
		cluster.Version = *expectedVersion + 1
		err = cluster.Validate(now)
		if err == nil {
			_, err = repository.Update(ctx, cluster, *expectedVersion, capacity, tenantIDs)
		}
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(os.Stdout, cluster.ID+" version="+strconv.FormatInt(cluster.Version, 10)); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

func splitValues(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func parseTenantIDs(value string) ([]uuid.UUID, error) {
	values := splitValues(value)
	identifiers := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		identifier, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant UUID")
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, nil
}
