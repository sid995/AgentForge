grant select (prompt_reference, max_attempts) on agent_runs to agentforge_scheduler;

create table agentrun_handoff_intents (
    event_id uuid primary key check (substr(event_id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    run_id uuid not null,
    attempt_id uuid not null check (substr(attempt_id::text, 15, 1) = '7'),
    attempt_number integer not null check (attempt_number between 1 and 10),
    cluster_id text not null check (cluster_id ~ '^[a-z0-9][a-z0-9-]{2,62}$'),
    desired_state jsonb not null check (jsonb_typeof(desired_state) = 'object'),
    created_at timestamptz not null,
    unique (tenant_id, run_id, attempt_number)
);

create index agentrun_handoff_intents_tenant_run_idx
    on agentrun_handoff_intents (tenant_id, run_id, attempt_number);

grant select, insert on agentrun_handoff_intents to agentforge_scheduler;

do $$
begin
    if not exists (select 1 from pg_roles where rolname = 'agentforge_handoff') then
        create role agentforge_handoff noinherit bypassrls nologin;
    end if;
end $$;

alter role agentforge_handoff noinherit bypassrls;
grant usage on schema public to agentforge_handoff;
grant select on clusters, cluster_tenant_allowlist to agentforge_handoff;
grant select on agentrun_handoff_intents to agentforge_handoff;
grant select, insert on processed_events to agentforge_handoff;
