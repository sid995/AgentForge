revoke all privileges on scheduler_eligibility_decisions from agentforge_scheduler;
revoke all privileges on tenant_daily_budget_usage from agentforge_scheduler;
revoke all privileges on scheduler_tenant_policies from agentforge_scheduler;
revoke select (id, tenant_id, status) on projects from agentforge_scheduler;
revoke select (id, status) on tenants from agentforge_scheduler;

drop table scheduler_eligibility_decisions;
drop table tenant_daily_budget_usage;
drop table scheduler_tenant_policies;

drop index agent_runs_scheduler_queued_quota_idx;
drop index agent_runs_scheduler_active_quota_idx;

alter table projects drop constraint projects_status_valid, drop column status;
alter table tenants drop constraint tenants_status_valid, drop column status;
