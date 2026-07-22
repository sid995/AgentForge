revoke all privileges on agent_run_attempts from agentforge_scheduler;
revoke all privileges on agent_runs from agentforge_scheduler;

drop index agent_runs_scheduler_queue_idx;

alter table agent_runs
    drop constraint agent_runs_scheduler_decision_reason_bounded,
    drop constraint agent_runs_scheduler_decision_code_bounded,
    drop constraint agent_runs_scheduler_lease_status,
    drop constraint agent_runs_scheduler_lease_owner_bounded,
    drop constraint agent_runs_scheduler_lease_consistent,
    drop constraint agent_runs_preferred_region_format,
    drop constraint agent_runs_execution_profile_format,
    drop constraint agent_runs_priority_bounded,
    drop column scheduler_decision_reason,
    drop column scheduler_decision_code,
    drop column scheduler_lease_expires_at,
    drop column scheduler_lease_owner,
    drop column scheduler_next_eligible_at,
    drop column preferred_region,
    drop column execution_profile,
    drop column priority;

create index agent_runs_queue_idx on agent_runs (created_at asc, id asc)
    where status in ('QUEUED', 'CAPACITY_WAIT');
