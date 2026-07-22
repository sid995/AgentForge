revoke all privileges on cluster_tenant_allowlist from agentforge_scheduler;
revoke all privileges on cluster_capacity_snapshots from agentforge_scheduler;
revoke all privileges on clusters from agentforge_scheduler;
drop table cluster_tenant_allowlist;
drop table cluster_capacity_snapshots;
drop table clusters;
