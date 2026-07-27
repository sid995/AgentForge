alter table agent_runs
    drop constraint agent_runs_last_command_consistent,
    drop column last_command_response,
    drop column last_command_id,
    drop column last_command_type;
