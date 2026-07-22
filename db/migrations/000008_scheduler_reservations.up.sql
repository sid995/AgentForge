create table capacity_reservations (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    run_id uuid not null,
    attempt_id uuid not null references agent_run_attempts (id) on delete restrict,
    attempt_number integer not null,
    cluster_id text not null references clusters (id) on delete restrict,
    cpu_millis bigint not null,
    memory_mib bigint not null,
    state text not null,
    expires_at timestamptz not null,
    created_at timestamptz not null,
    settled_at timestamptz,
    released_at timestamptz,
    constraint capacity_reservations_run_tenant_fk foreign key (run_id, tenant_id) references agent_runs (id, tenant_id) on delete restrict,
    constraint capacity_reservations_attempt_positive check (attempt_number > 0),
    constraint capacity_reservations_resources_positive check (cpu_millis > 0 and memory_mib > 0),
    constraint capacity_reservations_state_valid check (state in ('RESERVED', 'SETTLED', 'RELEASED', 'EXPIRED')),
    constraint capacity_reservations_lifecycle check (
        (state = 'RESERVED' and settled_at is null and released_at is null) or
        (state = 'SETTLED' and settled_at is not null and released_at is null) or
        (state in ('RELEASED', 'EXPIRED') and settled_at is null and released_at is not null)
    )
);

create unique index capacity_reservations_active_attempt_idx
    on capacity_reservations (run_id, attempt_number) where state = 'RESERVED';
create index capacity_reservations_cluster_active_idx
    on capacity_reservations (cluster_id, expires_at) include (cpu_millis, memory_mib)
    where state = 'RESERVED';

create table budget_reservations (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    capacity_reservation_id uuid not null unique references capacity_reservations (id) on delete restrict,
    tenant_id uuid not null references tenants (id) on delete restrict,
    run_id uuid not null,
    attempt_number integer not null,
    usage_date date not null,
    amount_minor_units bigint not null,
    state text not null,
    expires_at timestamptz not null,
    created_at timestamptz not null,
    settled_at timestamptz,
    released_at timestamptz,
    constraint budget_reservations_run_tenant_fk foreign key (run_id, tenant_id) references agent_runs (id, tenant_id) on delete restrict,
    constraint budget_reservations_amount_nonnegative check (amount_minor_units >= 0),
    constraint budget_reservations_attempt_positive check (attempt_number > 0),
    constraint budget_reservations_state_valid check (state in ('RESERVED', 'SETTLED', 'RELEASED', 'EXPIRED')),
    constraint budget_reservations_lifecycle check (
        (state = 'RESERVED' and settled_at is null and released_at is null) or
        (state = 'SETTLED' and settled_at is not null and released_at is null) or
        (state in ('RELEASED', 'EXPIRED') and settled_at is null and released_at is not null)
    )
);

create unique index budget_reservations_active_attempt_idx
    on budget_reservations (run_id, attempt_number) where state = 'RESERVED';
create index budget_reservations_tenant_expiry_idx
    on budget_reservations (tenant_id, expires_at) where state = 'RESERVED';

alter table capacity_reservations enable row level security;
alter table capacity_reservations force row level security;
alter table budget_reservations enable row level security;
alter table budget_reservations force row level security;

grant select, insert, update on capacity_reservations, budget_reservations to agentforge_scheduler;
grant insert, update (reserved_minor_units, spent_minor_units, updated_at) on tenant_daily_budget_usage to agentforge_scheduler;
