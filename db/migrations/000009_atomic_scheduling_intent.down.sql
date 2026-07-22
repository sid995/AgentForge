revoke insert on outbox_events from agentforge_scheduler;
revoke select (event_id, tenant_id, run_id, event_type, aggregate_version) on outbox_events from agentforge_scheduler;
revoke select (updated_at, scheduler_strategy, scheduler_selection_score, scheduler_decision_code, scheduler_decision_reason, scheduler_next_eligible_at) on agent_runs from agentforge_scheduler;
revoke update (status, version, updated_at, selected_cluster, execution_profile, capacity_reservation_id, budget_reservation_id) on agent_run_attempts from agentforge_scheduler;
revoke select (selected_cluster, execution_profile, capacity_reservation_id, budget_reservation_id) on agent_run_attempts from agentforge_scheduler;
revoke update (scheduler_next_eligible_at, scheduler_decision_code, scheduler_decision_reason, scheduler_strategy, scheduler_selection_score) on agent_runs from agentforge_scheduler;
alter table agent_runs
    drop constraint agent_runs_scheduler_selection_score_nonnegative,
    drop constraint agent_runs_scheduler_strategy_bounded,
    drop column scheduler_selection_score,
    drop column scheduler_strategy;
alter table agent_run_attempts
    drop constraint agent_run_attempts_scheduling_assignment,
    drop constraint agent_run_attempts_execution_profile_format,
    drop column budget_reservation_id,
    drop column capacity_reservation_id,
    drop column execution_profile;
