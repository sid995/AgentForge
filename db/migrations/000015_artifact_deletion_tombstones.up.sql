alter table artifacts
    add constraint artifacts_id_tenant_unique unique (id, tenant_id);

create table artifact_deletions (
    artifact_id uuid primary key,
    tenant_id uuid not null,
    deleted_by text not null,
    deletion_reason text not null,
    deleted_at timestamptz not null,
    constraint artifact_deletions_artifact_tenant_foreign_key foreign key (artifact_id, tenant_id)
        references artifacts (id, tenant_id) on delete restrict,
    constraint artifact_deletions_deleted_by_not_blank check (length(trim(deleted_by)) between 1 and 255),
    constraint artifact_deletions_reason_not_blank check (length(trim(deletion_reason)) between 1 and 500)
);

create index artifact_deletions_tenant_deleted_at_idx on artifact_deletions (tenant_id, deleted_at asc, artifact_id asc);

grant select, insert on artifact_deletions to agentforge_app;
alter table artifact_deletions enable row level security;
alter table artifact_deletions force row level security;

create policy artifact_deletions_tenant_isolation on artifact_deletions
    for all to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
