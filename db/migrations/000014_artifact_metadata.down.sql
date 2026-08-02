drop table artifacts;

alter table agent_run_attempts
    drop constraint agent_run_attempts_id_run_tenant_number_unique;

alter table agent_runs
    drop constraint agent_runs_id_project_tenant_unique;
