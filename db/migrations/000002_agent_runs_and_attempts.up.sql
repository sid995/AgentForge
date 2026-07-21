alter table projects add constraint projects_id_tenant_unique unique (id, tenant_id);

create table agent_runs (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    project_id uuid not null references projects (id) on delete restrict,
    prompt_reference text not null,
    runtime text not null,
    cpu_millis integer not null,
    memory_mib integer not null,
    timeout_seconds integer not null,
    max_attempts integer not null,
    attempt_count integer not null default 1,
    status text not null,
    failure_category text,
    idempotency_key text not null,
    request_hash text not null,
    version bigint not null default 1,
    created_by text not null,
    cancellation_reason text,
    cancellation_requested_by text,
    cancellation_requested_at timestamptz,
    created_at timestamptz not null,
    updated_at timestamptz not null,
    completed_at timestamptz,
    constraint agent_runs_project_tenant_unique foreign key (project_id, tenant_id) references projects (id, tenant_id),
    constraint agent_runs_id_tenant_unique unique (id, tenant_id),
    constraint agent_runs_prompt_reference_not_blank check (length(trim(prompt_reference)) between 1 and 2048),
    constraint agent_runs_runtime_not_blank check (length(trim(runtime)) between 1 and 120),
    constraint agent_runs_cpu_millis_positive check (cpu_millis between 1 and 128000),
    constraint agent_runs_memory_mib_positive check (memory_mib between 1 and 524288),
    constraint agent_runs_timeout_seconds_range check (timeout_seconds between 1 and 86400),
    constraint agent_runs_max_attempts_range check (max_attempts between 1 and 10),
    constraint agent_runs_attempt_count_range check (attempt_count between 1 and max_attempts),
    constraint agent_runs_status_valid check (status in ('QUEUED', 'SCHEDULING', 'CAPACITY_WAIT', 'PROVISIONING', 'RUNNING', 'TESTING', 'BUILDING', 'SCANNING', 'READY_TO_DEPLOY', 'DEPLOYING', 'CANCELLING', 'SUCCEEDED', 'CANCELLED', 'PROVISIONING_FAILED', 'EXECUTION_FAILED', 'BUILD_FAILED', 'POLICY_REJECTED', 'DEPLOYMENT_FAILED', 'TIMED_OUT')),
    constraint agent_runs_failure_category_valid check (failure_category is null or failure_category in ('VALIDATION', 'AUTHENTICATION', 'AUTHORIZATION', 'QUOTA', 'CONFLICT', 'TRANSIENT_DEPENDENCY', 'PERMANENT_DEPENDENCY', 'EXECUTION', 'POLICY', 'INTERNAL')),
    constraint agent_runs_failure_for_terminal_status check ((status in ('PROVISIONING_FAILED', 'EXECUTION_FAILED', 'BUILD_FAILED', 'POLICY_REJECTED', 'DEPLOYMENT_FAILED', 'TIMED_OUT')) = (failure_category is not null)),
    constraint agent_runs_version_positive check (version > 0),
    constraint agent_runs_idempotency_key_not_blank check (length(trim(idempotency_key)) between 1 and 255),
    constraint agent_runs_request_hash_not_blank check (length(trim(request_hash)) between 1 and 128),
    constraint agent_runs_created_by_not_blank check (length(trim(created_by)) between 1 and 255),
    constraint agent_runs_cancellation_metadata_consistent check ((status = 'CANCELLING' or status = 'CANCELLED') = (cancellation_requested_at is not null))
);

create unique index agent_runs_tenant_idempotency_key_unique on agent_runs (tenant_id, idempotency_key);
create index agent_runs_tenant_project_created_at_idx on agent_runs (tenant_id, project_id, created_at desc, id desc);
create index agent_runs_queue_idx on agent_runs (created_at asc, id asc) where status in ('QUEUED', 'CAPACITY_WAIT');

create table agent_run_attempts (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    run_id uuid not null,
    attempt_number integer not null,
    status text not null,
    failure_category text,
    version bigint not null default 1,
    selected_cluster text,
    workload_reference text,
    created_at timestamptz not null,
    updated_at timestamptz not null,
    started_at timestamptz,
    completed_at timestamptz,
    constraint agent_run_attempts_run_tenant_foreign_key foreign key (run_id, tenant_id) references agent_runs (id, tenant_id) on delete restrict,
    constraint agent_run_attempts_number_positive check (attempt_number > 0),
    constraint agent_run_attempts_status_valid check (status in ('PENDING', 'STARTING', 'ACTIVE', 'COMPLETED', 'FAILED', 'CANCELLED', 'TIMED_OUT')),
    constraint agent_run_attempts_failure_category_valid check (failure_category is null or failure_category in ('VALIDATION', 'AUTHENTICATION', 'AUTHORIZATION', 'QUOTA', 'CONFLICT', 'TRANSIENT_DEPENDENCY', 'PERMANENT_DEPENDENCY', 'EXECUTION', 'POLICY', 'INTERNAL')),
    constraint agent_run_attempts_version_positive check (version > 0),
    constraint agent_run_attempts_unique_number unique (run_id, attempt_number)
);

create index agent_run_attempts_active_idx on agent_run_attempts (run_id, attempt_number desc) where status in ('PENDING', 'STARTING', 'ACTIVE');

grant select, insert, update on agent_runs, agent_run_attempts to agentforge_app;
alter table agent_runs enable row level security;
alter table agent_runs force row level security;
alter table agent_run_attempts enable row level security;
alter table agent_run_attempts force row level security;

create policy agent_runs_tenant_isolation on agent_runs
    for all to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

create policy agent_run_attempts_tenant_isolation on agent_run_attempts
    for all to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
