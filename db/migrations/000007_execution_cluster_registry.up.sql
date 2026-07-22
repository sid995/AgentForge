create table clusters (
    id text primary key,
    region text not null,
    status text not null,
    supported_runtimes text[] not null,
    supported_execution_profiles text[] not null,
    maintenance boolean not null default false,
    tenant_restricted boolean not null default false,
    scheduling_weight integer not null default 100,
    cpu_cost_minor_units integer not null default 0,
    memory_gib_cost_minor_units integer not null default 0,
    last_heartbeat_at timestamptz not null,
    version bigint not null default 1,
    created_at timestamptz not null,
    updated_at timestamptz not null,
    constraint clusters_id_format check (id ~ '^[a-z0-9][a-z0-9-]{2,62}$'),
    constraint clusters_region_format check (region ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    constraint clusters_status_valid check (status in ('ACTIVE', 'UNAVAILABLE')),
    constraint clusters_runtimes_bounded check (cardinality(supported_runtimes) between 1 and 64 and array_position(supported_runtimes, null) is null),
    constraint clusters_profiles_bounded check (cardinality(supported_execution_profiles) between 1 and 32 and array_position(supported_execution_profiles, null) is null),
    constraint clusters_weight_bounded check (scheduling_weight between 1 and 1000),
    constraint clusters_cost_nonnegative check (cpu_cost_minor_units >= 0 and memory_gib_cost_minor_units >= 0),
    constraint clusters_version_positive check (version > 0)
);

create table cluster_capacity_snapshots (
    cluster_id text not null references clusters (id) on delete restrict,
    observed_at timestamptz not null,
    allocatable_cpu_millis bigint not null,
    allocatable_memory_mib bigint not null,
    queued_workloads integer not null,
    primary key (cluster_id, observed_at),
    constraint cluster_capacity_resources_nonnegative check (allocatable_cpu_millis >= 0 and allocatable_memory_mib >= 0 and queued_workloads >= 0)
);

create table cluster_tenant_allowlist (
    cluster_id text not null references clusters (id) on delete cascade,
    tenant_id uuid not null references tenants (id) on delete restrict,
    primary key (cluster_id, tenant_id)
);

create index clusters_eligible_idx on clusters (status, maintenance, region, id);
create index cluster_capacity_latest_idx on cluster_capacity_snapshots (cluster_id, observed_at desc);
create index cluster_tenant_allowlist_tenant_idx on cluster_tenant_allowlist (tenant_id, cluster_id);

alter table cluster_tenant_allowlist enable row level security;
alter table cluster_tenant_allowlist force row level security;

grant select, insert, update on clusters to agentforge_scheduler;
grant select, insert on cluster_capacity_snapshots to agentforge_scheduler;
grant select, insert, delete on cluster_tenant_allowlist to agentforge_scheduler;
