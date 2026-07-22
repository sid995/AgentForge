alter table agent_run_attempts
    add column execution_profile text,
    add column capacity_reservation_id uuid references capacity_reservations (id) on delete restrict,
    add column budget_reservation_id uuid references budget_reservations (id) on delete restrict,
    add constraint agent_run_attempts_execution_profile_format check (execution_profile is null or execution_profile ~ '^[a-z0-9][a-z0-9._-]{0,62}$'),
    add constraint agent_run_attempts_scheduling_assignment check (
        (selected_cluster is null and execution_profile is null and capacity_reservation_id is null and budget_reservation_id is null) or
        (selected_cluster is not null and execution_profile is not null and capacity_reservation_id is not null and budget_reservation_id is not null)
    );

alter table agent_runs
    add column scheduler_strategy text,
    add column scheduler_selection_score bigint,
    add constraint agent_runs_scheduler_strategy_bounded check (scheduler_strategy is null or length(trim(scheduler_strategy)) between 1 and 80),
    add constraint agent_runs_scheduler_selection_score_nonnegative check (scheduler_selection_score is null or scheduler_selection_score >= 0);

grant select (updated_at, scheduler_strategy, scheduler_selection_score, scheduler_decision_code, scheduler_decision_reason, scheduler_next_eligible_at) on agent_runs to agentforge_scheduler;

grant update (
    status, version, updated_at, scheduler_lease_owner, scheduler_lease_expires_at,
    scheduler_next_eligible_at, scheduler_decision_code, scheduler_decision_reason,
    scheduler_strategy, scheduler_selection_score
) on agent_runs to agentforge_scheduler;
grant update (
    status, version, updated_at, selected_cluster, execution_profile,
    capacity_reservation_id, budget_reservation_id
) on agent_run_attempts to agentforge_scheduler;
grant select (selected_cluster, execution_profile, capacity_reservation_id, budget_reservation_id) on agent_run_attempts to agentforge_scheduler;
grant insert on outbox_events to agentforge_scheduler;
grant select (event_id, tenant_id, run_id, event_type, aggregate_version) on outbox_events to agentforge_scheduler;
