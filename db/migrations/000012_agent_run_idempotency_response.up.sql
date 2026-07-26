alter table agent_runs
    add column idempotency_response jsonb not null default '{}'::jsonb;

comment on column agent_runs.idempotency_response is
    'Safe immutable create-run replay response; excludes prompt references, hashes, and actor data.';
