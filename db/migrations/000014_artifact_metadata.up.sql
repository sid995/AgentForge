alter table agent_runs
    add constraint agent_runs_id_project_tenant_unique unique (id, project_id, tenant_id);

alter table agent_run_attempts
    add constraint agent_run_attempts_id_run_tenant_number_unique unique (id, run_id, tenant_id, attempt_number);

create table artifacts (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    project_id uuid not null,
    run_id uuid not null,
    attempt_id uuid not null,
    attempt_number integer not null,
    object_key text not null,
    content_type text not null,
    size_bytes bigint not null,
    sha256 text not null,
    retention_class text not null,
    created_by text not null,
    created_at timestamptz not null,
    constraint artifacts_run_project_tenant_foreign_key foreign key (run_id, project_id, tenant_id)
        references agent_runs (id, project_id, tenant_id) on delete restrict,
    constraint artifacts_attempt_run_tenant_number_foreign_key foreign key (attempt_id, run_id, tenant_id, attempt_number)
        references agent_run_attempts (id, run_id, tenant_id, attempt_number) on delete restrict,
    constraint artifacts_object_key_not_blank check (length(trim(object_key)) between 1 and 2048),
    constraint artifacts_content_type_not_blank check (length(trim(content_type)) between 1 and 255),
    constraint artifacts_size_non_negative check (size_bytes >= 0),
    constraint artifacts_sha256_valid check (sha256 ~ '^[0-9a-f]{64}$'),
    constraint artifacts_retention_class_valid check (retention_class in ('HOT', 'ARCHIVE')),
    constraint artifacts_created_by_not_blank check (length(trim(created_by)) between 1 and 255),
    constraint artifacts_object_key_unique unique (object_key)
);

create index artifacts_tenant_run_created_at_idx on artifacts (tenant_id, run_id, created_at asc, id asc);
create index artifacts_tenant_attempt_idx on artifacts (tenant_id, attempt_id);

grant select, insert on artifacts to agentforge_app;
alter table artifacts enable row level security;
alter table artifacts force row level security;

create policy artifacts_tenant_isolation on artifacts
    for all to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
