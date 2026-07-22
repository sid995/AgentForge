alter table tenants
    add column status text not null default 'ACTIVE',
    add constraint tenants_status_valid check (status in ('ACTIVE', 'SUSPENDED'));

alter table projects
    add column status text not null default 'ACTIVE',
    add constraint projects_status_valid check (status in ('ACTIVE', 'SUSPENDED'));

create table scheduler_tenant_policies (
    tenant_id uuid primary key references tenants (id) on delete restrict,
    max_concurrent_runs integer not null,
    max_user_concurrent_runs integer not null,
    max_queued_runs integer not null,
    max_cpu_millis bigint not null,
    max_memory_mib bigint not null,
    daily_budget_minor_units bigint not null,
    allowed_runtimes text[] not null,
    allowed_execution_profiles text[] not null,
    deferral_seconds integer not null default 30,
    updated_at timestamptz not null,
    constraint scheduler_policy_concurrency_positive check (max_concurrent_runs > 0 and max_user_concurrent_runs > 0 and max_user_concurrent_runs <= max_concurrent_runs),
    constraint scheduler_policy_queue_positive check (max_queued_runs > 0),
    constraint scheduler_policy_resources_positive check (max_cpu_millis > 0 and max_memory_mib > 0),
    constraint scheduler_policy_budget_nonnegative check (daily_budget_minor_units >= 0),
    constraint scheduler_policy_runtimes_bounded check (cardinality(allowed_runtimes) between 1 and 64 and array_position(allowed_runtimes, null) is null),
    constraint scheduler_policy_profiles_bounded check (cardinality(allowed_execution_profiles) between 1 and 32 and array_position(allowed_execution_profiles, null) is null),
    constraint scheduler_policy_deferral_bounded check (deferral_seconds between 1 and 3600)
);

create table tenant_daily_budget_usage (
    tenant_id uuid not null references tenants (id) on delete restrict,
    usage_date date not null,
    spent_minor_units bigint not null default 0,
    reserved_minor_units bigint not null default 0,
    updated_at timestamptz not null,
    primary key (tenant_id, usage_date),
    constraint tenant_daily_budget_spent_nonnegative check (spent_minor_units >= 0),
    constraint tenant_daily_budget_reserved_nonnegative check (reserved_minor_units >= 0)
);

create table scheduler_eligibility_decisions (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    run_id uuid not null,
    attempt_number integer not null,
    run_version bigint not null,
    outcome text not null,
    reason_code text not null,
    explanation text not null,
    active_tenant_runs integer not null,
    active_user_runs integer not null,
    queued_tenant_runs integer not null,
    active_cpu_millis bigint not null,
    active_memory_mib bigint not null,
    budget_used_minor_units bigint not null,
    next_eligible_at timestamptz,
    evaluated_at timestamptz not null,
    constraint scheduler_decision_run_tenant_fk foreign key (run_id, tenant_id) references agent_runs (id, tenant_id) on delete restrict,
    constraint scheduler_decision_attempt_positive check (attempt_number > 0),
    constraint scheduler_decision_version_positive check (run_version > 0),
    constraint scheduler_decision_outcome_valid check (outcome in ('ELIGIBLE', 'DEFER', 'REJECT')),
    constraint scheduler_decision_reason_bounded check (length(trim(reason_code)) between 1 and 80),
    constraint scheduler_decision_explanation_bounded check (length(trim(explanation)) between 1 and 500),
    constraint scheduler_decision_counts_nonnegative check (active_tenant_runs >= 0 and active_user_runs >= 0 and queued_tenant_runs >= 0 and active_cpu_millis >= 0 and active_memory_mib >= 0 and budget_used_minor_units >= 0),
    constraint scheduler_decision_next_eligible_consistent check ((outcome = 'DEFER') = (next_eligible_at is not null))
);

create index agent_runs_scheduler_active_quota_idx
    on agent_runs (tenant_id, created_by, status)
    include (cpu_millis, memory_mib)
    where status in ('PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING');
create index agent_runs_scheduler_queued_quota_idx
    on agent_runs (tenant_id, status)
    where status in ('QUEUED', 'SCHEDULING', 'CAPACITY_WAIT');
create index scheduler_eligibility_decisions_run_time_idx
    on scheduler_eligibility_decisions (tenant_id, run_id, evaluated_at desc, id desc);

alter table scheduler_tenant_policies enable row level security;
alter table scheduler_tenant_policies force row level security;
alter table tenant_daily_budget_usage enable row level security;
alter table tenant_daily_budget_usage force row level security;
alter table scheduler_eligibility_decisions enable row level security;
alter table scheduler_eligibility_decisions force row level security;

grant select (id, status) on tenants to agentforge_scheduler;
grant select (id, tenant_id, status) on projects to agentforge_scheduler;
grant select, update (updated_at) on scheduler_tenant_policies to agentforge_scheduler;
grant select on tenant_daily_budget_usage to agentforge_scheduler;
grant select, insert on scheduler_eligibility_decisions to agentforge_scheduler;
