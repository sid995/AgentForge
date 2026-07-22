//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	postgresadapter "github.com/sid995/agentforge/services/platform-api/internal/adapters/postgres"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
)

func TestClusterRegistryFiltersCandidatesAndProtectsVersions(t *testing.T) {
	ctx := context.Background()
	migrationURL := requiredEnvironment(t, "AGENTFORGE_TEST_DATABASE_URL")
	schedulerURL := requiredEnvironment(t, "AGENTFORGE_TEST_SCHEDULER_DATABASE_URL")
	if err := migrations.Apply(migrationURL, migrationDirectory(t)); err != nil {
		t.Fatal(err)
	}
	adminPool := openPool(t, migrationURL, 3)
	defer adminPool.Close()
	schedulerPool := openPool(t, schedulerURL, 3)
	defer schedulerPool.Close()
	now := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	tenantA := mustTenant(t, uniqueSlug("cluster-a"), now)
	tenantB := mustTenant(t, uniqueSlug("cluster-b"), now)
	tenantRepository := postgresadapter.NewTenantRepository(adminPool)
	for _, tenant := range []domain.Tenant{tenantA, tenantB} {
		if err := tenantRepository.Create(ctx, tenant); err != nil {
			t.Fatal(err)
		}
	}
	registry := postgresadapter.NewClusterRegistry(schedulerPool)
	register := func(id string, heartbeat time.Time, maintenance, restricted bool, runtimes []string, allowed []uuid.UUID) domain.ExecutionCluster {
		t.Helper()
		cluster, err := domain.NewExecutionCluster(domain.ExecutionCluster{
			ID: id, Region: "us-east-1", Status: domain.ClusterActive,
			SupportedRuntimes: runtimes, SupportedExecutionProfiles: []string{"standard"},
			Maintenance: maintenance, TenantRestricted: restricted, SchedulingWeight: 100,
			LastHeartbeatAt: heartbeat,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		capacity := domain.ClusterCapacity{ClusterID: id, ObservedAt: heartbeat, AllocatableCPUMillis: 4000, AllocatableMemoryMiB: 8192}
		if err := registry.Register(ctx, cluster, capacity, allowed); err != nil {
			t.Fatal(err)
		}
		return cluster
	}
	active := register("cluster-active", now, false, true, []string{"python-3.12"}, []uuid.UUID{tenantA.ID})
	register("cluster-stale", now.Add(-10*time.Minute), false, false, []string{"python-3.12"}, nil)
	register("cluster-maintenance", now, true, false, []string{"python-3.12"}, nil)
	register("cluster-runtime", now, false, false, []string{"go-1.26"}, nil)

	candidates, err := registry.ListCandidates(ctx, tenantA.ID, "python-3.12", "standard", now, 5*time.Minute)
	if err != nil || len(candidates) != 1 || candidates[0].Cluster.ID != active.ID {
		t.Fatalf("tenant A candidates=%#v err=%v", candidates, err)
	}
	candidates, err = registry.ListCandidates(ctx, tenantB.ID, "python-3.12", "standard", now, 5*time.Minute)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("tenant B candidates=%#v err=%v", candidates, err)
	}
	stored, capacity, err := registry.Get(ctx, active.ID)
	if err != nil || stored.ID != active.ID || capacity.ClusterID != active.ID {
		t.Fatalf("get cluster=%#v capacity=%#v err=%v", stored, capacity, err)
	}
	updated := stored
	updated.Version++
	updated.UpdatedAt = now.Add(time.Second)
	updated.LastHeartbeatAt = now.Add(time.Second)
	newCapacity := capacity
	newCapacity.ObservedAt = now.Add(time.Second)
	newCapacity.AllocatableCPUMillis = 5000
	if _, err := registry.Update(ctx, updated, stored.Version, newCapacity, []uuid.UUID{tenantA.ID, tenantB.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Update(ctx, updated, stored.Version, newCapacity, []uuid.UUID{tenantA.ID}); !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale update error=%v", err)
	}
	candidates, err = registry.ListCandidates(ctx, tenantB.ID, "python-3.12", "standard", now.Add(time.Second), 5*time.Minute)
	if err != nil || len(candidates) != 1 || candidates[0].Capacity.AllocatableCPUMillis != 5000 {
		t.Fatalf("updated candidates=%#v err=%v", candidates, err)
	}
}
