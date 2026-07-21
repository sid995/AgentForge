do $$
begin
    if not exists (select 1 from pg_roles where rolname = 'agentforge_relay') then
        create role agentforge_relay noinherit bypassrls nologin;
    end if;
end $$;

alter role agentforge_relay noinherit bypassrls;

create table outbox_events (
    event_id uuid primary key check (substr(event_id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    project_id uuid,
    run_id uuid,
    event_type text not null,
    schema_version integer not null,
    aggregate_type text not null,
    aggregate_id uuid not null,
    aggregate_version bigint not null,
    topic text not null,
    partition_key text not null,
    envelope jsonb not null,
    disposition text not null default 'PENDING',
    next_attempt_at timestamptz not null,
    publication_attempts integer not null default 0,
    claimed_by text,
    claim_expires_at timestamptz,
    published_at timestamptz,
    broker_partition integer,
    broker_offset bigint,
    last_failure_category text,
    last_failure_reason text,
    last_failure_at timestamptz,
    created_at timestamptz not null,
    constraint outbox_event_type_not_blank check (length(trim(event_type)) between 1 and 160),
    constraint outbox_schema_version_positive check (schema_version > 0),
    constraint outbox_aggregate_version_positive check (aggregate_version > 0),
    constraint outbox_topic_not_blank check (length(trim(topic)) between 1 and 249),
    constraint outbox_partition_key_not_blank check (length(trim(partition_key)) between 1 and 255),
    constraint outbox_disposition_valid check (disposition in ('PENDING', 'PUBLISHED', 'TERMINAL')),
    constraint outbox_attempts_nonnegative check (publication_attempts >= 0),
    constraint outbox_claim_consistent check ((claimed_by is null) = (claim_expires_at is null)),
    constraint outbox_publish_consistent check ((disposition = 'PUBLISHED') = (published_at is not null)),
    constraint outbox_failure_reason_bounded check (last_failure_reason is null or length(last_failure_reason) <= 1000)
);

create index outbox_events_claimable_idx on outbox_events (next_attempt_at, claim_expires_at, created_at, event_id)
    where disposition = 'PENDING' and published_at is null;
create index outbox_events_published_cleanup_idx on outbox_events (published_at, event_id)
    where disposition = 'PUBLISHED';

grant insert on outbox_events to agentforge_app;
grant select, update, delete on outbox_events to agentforge_relay;

alter table outbox_events enable row level security;
alter table outbox_events force row level security;

create policy outbox_events_app_insert on outbox_events
    for insert to agentforge_app
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
