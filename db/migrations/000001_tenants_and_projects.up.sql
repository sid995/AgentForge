create table tenants (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    slug text not null unique,
    name text not null,
    created_at timestamptz not null,
    updated_at timestamptz not null,
    constraint tenants_slug_format check (slug ~ '^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$'),
    constraint tenants_name_not_blank check (length(trim(name)) between 1 and 120)
);

create table projects (
    id uuid primary key check (substr(id::text, 15, 1) = '7'),
    tenant_id uuid not null references tenants (id) on delete restrict,
    name text not null,
    repository_url text not null default '',
    version bigint not null default 1,
    created_at timestamptz not null,
    updated_at timestamptz not null,
    constraint projects_name_not_blank check (length(trim(name)) between 1 and 120),
    constraint projects_repository_url_length check (length(repository_url) <= 2048),
    constraint projects_version_positive check (version > 0),
    constraint projects_tenant_name_unique unique (tenant_id, name)
);

create index projects_tenant_created_at_idx on projects (tenant_id, created_at desc, id desc);

do $$
begin
    if not exists (select 1 from pg_roles where rolname = 'agentforge_app') then
        create role agentforge_app noinherit nobypassrls nologin;
    end if;
end $$;

alter role agentforge_app noinherit nobypassrls;
grant usage on schema public to agentforge_app;
grant select, insert, update on projects to agentforge_app;

alter table projects enable row level security;
alter table projects force row level security;

create policy projects_tenant_isolation on projects
    for all to agentforge_app
    using (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    with check (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
