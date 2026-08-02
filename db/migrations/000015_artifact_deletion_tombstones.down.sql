drop table artifact_deletions;

alter table artifacts
    drop constraint artifacts_id_tenant_unique;
