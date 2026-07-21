create table processed_events (
    consumer_identity text not null,
    event_id uuid not null check (substr(event_id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    event_type text not null,
    schema_version integer not null,
    aggregate_type text not null,
    aggregate_id uuid not null,
    aggregate_version bigint not null,
    source_topic text not null,
    source_partition integer not null,
    source_offset bigint not null,
    processed_at timestamptz not null,
    replay_id text,
    primary key (consumer_identity, event_id),
    constraint processed_consumer_not_blank check (length(trim(consumer_identity)) between 1 and 160),
    constraint processed_event_type_not_blank check (length(trim(event_type)) between 1 and 160),
    constraint processed_schema_version_positive check (schema_version > 0),
    constraint processed_aggregate_version_positive check (aggregate_version > 0),
    constraint processed_source_topic_not_blank check (length(trim(source_topic)) between 1 and 249),
    constraint processed_source_partition_nonnegative check (source_partition >= 0),
    constraint processed_source_offset_nonnegative check (source_offset >= 0),
    constraint processed_replay_id_bounded check (replay_id is null or length(replay_id) between 1 and 255)
);

create index processed_events_tenant_time_idx on processed_events (tenant_id, processed_at, event_id);

grant select, insert on processed_events to agentforge_app;

alter table processed_events enable row level security;
alter table processed_events force row level security;

create policy processed_events_app_select on processed_events
    for select to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
create policy processed_events_app_insert on processed_events
    for insert to agentforge_app
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
