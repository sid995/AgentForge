do $$
begin
    if not exists (select 1 from pg_roles where rolname = 'agentforge_scheduler') then
        create role agentforge_scheduler noinherit bypassrls nologin;
    end if;
end $$;

alter role agentforge_scheduler noinherit bypassrls;
grant usage on schema public to agentforge_scheduler;

alter table agent_runs
    add column priority smallint not null default 0,
    add column execution_profile text not null default 'standard',
    add column preferred_region text,
    add column scheduler_next_eligible_at timestamptz,
    add column scheduler_lease_owner text,
    add column scheduler_lease_expires_at timestamptz,
    add column scheduler_decision_code text,
    add column scheduler_decision_reason text,
    add constraint agent_runs_priority_bounded check (priority between -100 and 100),
    add constraint agent_runs_execution_profile_format check (execution_profile ~ '^[a-z0-9][a-z0-9._-]{0,62}$'),
    add constraint agent_runs_preferred_region_format check (preferred_region is null or preferred_region ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    add constraint agent_runs_scheduler_lease_consistent check ((scheduler_lease_owner is null) = (scheduler_lease_expires_at is null)),
    add constraint agent_runs_scheduler_lease_owner_bounded check (scheduler_lease_owner is null or length(trim(scheduler_lease_owner)) between 1 and 160),
    add constraint agent_runs_scheduler_lease_status check ((status = 'SCHEDULING') = (scheduler_lease_owner is not null)),
    add constraint agent_runs_scheduler_decision_code_bounded check (scheduler_decision_code is null or length(trim(scheduler_decision_code)) between 1 and 80),
    add constraint agent_runs_scheduler_decision_reason_bounded check (scheduler_decision_reason is null or length(trim(scheduler_decision_reason)) between 1 and 500);

drop index agent_runs_queue_idx;
create index agent_runs_scheduler_queue_idx
    on agent_runs (priority desc, created_at asc, id asc)
    include (tenant_id, version, scheduler_next_eligible_at, scheduler_lease_expires_at)
    where status in ('QUEUED', 'CAPACITY_WAIT', 'SCHEDULING');

grant select (
    id, tenant_id, project_id, runtime, cpu_millis, memory_mib,
    timeout_seconds, attempt_count, status, version, created_by, created_at,
    priority, execution_profile, preferred_region, scheduler_next_eligible_at,
    scheduler_lease_owner, scheduler_lease_expires_at
) on agent_runs to agentforge_scheduler;
grant update (
    status, version, updated_at, scheduler_lease_owner,
    scheduler_lease_expires_at
) on agent_runs to agentforge_scheduler;
grant select (id, tenant_id, run_id, attempt_number, status, version)
    on agent_run_attempts to agentforge_scheduler;
