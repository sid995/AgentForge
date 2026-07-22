package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sid995/agentforge/services/platform-api/internal/database"
	"github.com/sid995/agentforge/services/platform-api/internal/domain"
	"github.com/sid995/agentforge/services/platform-api/internal/ports"
)

type ClusterRegistry struct{ database *database.Pool }

func NewClusterRegistry(pool *database.Pool) *ClusterRegistry {
	return &ClusterRegistry{database: pool}
}

func (repository *ClusterRegistry) Register(ctx context.Context, cluster domain.ExecutionCluster, capacity domain.ClusterCapacity, allowedTenants []uuid.UUID) error {
	if err := validateClusterWrite(cluster, capacity, allowedTenants, cluster.UpdatedAt); err != nil {
		return err
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `
		insert into clusters (id, region, status, supported_runtimes, supported_execution_profiles,
			maintenance, tenant_restricted, scheduling_weight, cpu_cost_minor_units,
			memory_gib_cost_minor_units, last_heartbeat_at, version, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		cluster.ID, cluster.Region, cluster.Status, cluster.SupportedRuntimes,
		cluster.SupportedExecutionProfiles, cluster.Maintenance, cluster.TenantRestricted,
		cluster.SchedulingWeight, cluster.CPUCostMinorUnits, cluster.MemoryGiBCostMinorUnits,
		cluster.LastHeartbeatAt, cluster.Version, cluster.CreatedAt, cluster.UpdatedAt); err != nil {
		return translateError(err)
	}
	if err := writeCapacityAndAllowlist(ctx, tx, capacity, allowedTenants); err != nil {
		return err
	}
	return translateError(tx.Commit())
}

func (repository *ClusterRegistry) Update(ctx context.Context, cluster domain.ExecutionCluster, expectedVersion int64, capacity domain.ClusterCapacity, allowedTenants []uuid.UUID) (domain.ExecutionCluster, error) {
	if expectedVersion < 1 || cluster.Version != expectedVersion+1 {
		return domain.ExecutionCluster{}, domain.ErrVersionConflict
	}
	if err := validateClusterWrite(cluster, capacity, allowedTenants, cluster.UpdatedAt); err != nil {
		return domain.ExecutionCluster{}, err
	}
	tx, err := repository.database.Raw().BeginTx(ctx, nil)
	if err != nil {
		return domain.ExecutionCluster{}, translateError(err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		update clusters set region=$2, status=$3, supported_runtimes=$4,
			supported_execution_profiles=$5, maintenance=$6, tenant_restricted=$7,
			scheduling_weight=$8, cpu_cost_minor_units=$9, memory_gib_cost_minor_units=$10,
			last_heartbeat_at=$11, version=$12, updated_at=$13
		where id=$1 and version=$14`, cluster.ID, cluster.Region, cluster.Status,
		cluster.SupportedRuntimes, cluster.SupportedExecutionProfiles, cluster.Maintenance,
		cluster.TenantRestricted, cluster.SchedulingWeight, cluster.CPUCostMinorUnits,
		cluster.MemoryGiBCostMinorUnits, cluster.LastHeartbeatAt, cluster.Version,
		cluster.UpdatedAt, expectedVersion)
	if err != nil {
		return domain.ExecutionCluster{}, translateError(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ExecutionCluster{}, translateError(err)
	}
	if rows == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from clusters where id=$1)`, cluster.ID).Scan(&exists); err != nil {
			return domain.ExecutionCluster{}, translateError(err)
		}
		if !exists {
			return domain.ExecutionCluster{}, domain.ErrNotFound
		}
		return domain.ExecutionCluster{}, domain.ErrVersionConflict
	}
	if err := writeCapacityAndAllowlist(ctx, tx, capacity, allowedTenants); err != nil {
		return domain.ExecutionCluster{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ExecutionCluster{}, translateError(err)
	}
	return cluster, nil
}

func (repository *ClusterRegistry) Get(ctx context.Context, id string) (domain.ExecutionCluster, domain.ClusterCapacity, error) {
	row := repository.database.Raw().QueryRowContext(ctx, clusterSelect+`
		where cluster.id=$1 order by capacity.observed_at desc limit 1`, id)
	cluster, capacity, err := scanCluster(row)
	return cluster, capacity, translateError(err)
}

func (repository *ClusterRegistry) ListCandidates(ctx context.Context, tenantID uuid.UUID, runtime, profile string, now time.Time, freshness time.Duration) ([]domain.ClusterCandidate, error) {
	if tenantID == uuid.Nil || runtime == "" || profile == "" || now.IsZero() || freshness <= 0 {
		return nil, fmt.Errorf("candidate query is invalid")
	}
	rows, err := repository.database.Raw().QueryContext(ctx, clusterSelect+`
		where cluster.status='ACTIVE' and not cluster.maintenance
		  and cluster.last_heartbeat_at >= $2
		  and $3 = any(cluster.supported_runtimes)
		  and $4 = any(cluster.supported_execution_profiles)
		  and (not cluster.tenant_restricted or exists (
			select 1 from cluster_tenant_allowlist allowed
			where allowed.cluster_id=cluster.id and allowed.tenant_id=$1))
		  and capacity.observed_at >= $2
		order by cluster.id`, tenantID, now.UTC().Add(-freshness), runtime, profile)
	if err != nil {
		return nil, translateError(err)
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]domain.ClusterCandidate, 0)
	for rows.Next() {
		cluster, capacity, scanErr := scanCluster(rows)
		if scanErr != nil {
			return nil, translateError(scanErr)
		}
		candidates = append(candidates, domain.ClusterCandidate{Cluster: cluster, Capacity: capacity})
	}
	return candidates, translateError(rows.Err())
}

const clusterSelect = `
	select cluster.id, cluster.region, cluster.status, array_to_json(cluster.supported_runtimes),
		array_to_json(cluster.supported_execution_profiles), cluster.maintenance,
		cluster.tenant_restricted, cluster.scheduling_weight, cluster.cpu_cost_minor_units,
		cluster.memory_gib_cost_minor_units, cluster.last_heartbeat_at, cluster.version,
		cluster.created_at, cluster.updated_at, capacity.observed_at,
		capacity.allocatable_cpu_millis, capacity.allocatable_memory_mib, capacity.queued_workloads
	from clusters cluster
	join lateral (
		select observed_at, allocatable_cpu_millis, allocatable_memory_mib, queued_workloads
		from cluster_capacity_snapshots where cluster_id=cluster.id
		order by observed_at desc limit 1
	) capacity on true`

func validateClusterWrite(cluster domain.ExecutionCluster, capacity domain.ClusterCapacity, allowedTenants []uuid.UUID, now time.Time) error {
	if err := cluster.Validate(now); err != nil {
		return err
	}
	if err := capacity.Validate(now); err != nil || capacity.ClusterID != cluster.ID {
		return fmt.Errorf("cluster capacity does not match cluster metadata")
	}
	seen := make(map[uuid.UUID]bool, len(allowedTenants))
	for _, tenantID := range allowedTenants {
		if tenantID == uuid.Nil || seen[tenantID] {
			return fmt.Errorf("cluster tenant allowlist is invalid")
		}
		seen[tenantID] = true
	}
	if cluster.TenantRestricted && len(allowedTenants) == 0 {
		return fmt.Errorf("restricted cluster requires an allowlist")
	}
	return nil
}

func writeCapacityAndAllowlist(ctx context.Context, tx *sql.Tx, capacity domain.ClusterCapacity, allowedTenants []uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `insert into cluster_capacity_snapshots (cluster_id, observed_at, allocatable_cpu_millis, allocatable_memory_mib, queued_workloads) values ($1,$2,$3,$4,$5)`, capacity.ClusterID, capacity.ObservedAt, capacity.AllocatableCPUMillis, capacity.AllocatableMemoryMiB, capacity.QueuedWorkloads); err != nil {
		return translateError(err)
	}
	if _, err := tx.ExecContext(ctx, `delete from cluster_tenant_allowlist where cluster_id=$1`, capacity.ClusterID); err != nil {
		return translateError(err)
	}
	for _, tenantID := range allowedTenants {
		if _, err := tx.ExecContext(ctx, `insert into cluster_tenant_allowlist (cluster_id, tenant_id) values ($1,$2)`, capacity.ClusterID, tenantID); err != nil {
			return translateError(err)
		}
	}
	return nil
}

func scanCluster(row rowScanner) (domain.ExecutionCluster, domain.ClusterCapacity, error) {
	var cluster domain.ExecutionCluster
	var capacity domain.ClusterCapacity
	var runtimes, profiles []byte
	err := row.Scan(&cluster.ID, &cluster.Region, &cluster.Status, &runtimes, &profiles,
		&cluster.Maintenance, &cluster.TenantRestricted, &cluster.SchedulingWeight,
		&cluster.CPUCostMinorUnits, &cluster.MemoryGiBCostMinorUnits, &cluster.LastHeartbeatAt,
		&cluster.Version, &cluster.CreatedAt, &cluster.UpdatedAt, &capacity.ObservedAt,
		&capacity.AllocatableCPUMillis, &capacity.AllocatableMemoryMiB, &capacity.QueuedWorkloads)
	if err != nil {
		return cluster, capacity, err
	}
	capacity.ClusterID = cluster.ID
	if err := json.Unmarshal(runtimes, &cluster.SupportedRuntimes); err != nil {
		return cluster, capacity, err
	}
	if err := json.Unmarshal(profiles, &cluster.SupportedExecutionProfiles); err != nil {
		return cluster, capacity, err
	}
	return cluster, capacity, nil
}

var _ ports.ClusterRegistry = (*ClusterRegistry)(nil)
