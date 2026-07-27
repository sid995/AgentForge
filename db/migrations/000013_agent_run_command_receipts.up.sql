alter table agent_runs
    add column last_command_type text,
    add column last_command_id text,
    add column last_command_response jsonb,
    add constraint agent_runs_last_command_consistent check (
        (last_command_type is null and last_command_id is null and last_command_response is null) or
        (last_command_type in ('CANCEL', 'RETRY') and length(trim(last_command_id)) between 1 and 255 and last_command_response is not null)
    );

comment on column agent_runs.last_command_response is
    'Safe immutable response for the latest cancel or retry command; excludes prompt references, hashes, and actor data.';
