revoke all privileges on tenant_daily_budget_usage from agentforge_scheduler;
grant select on tenant_daily_budget_usage to agentforge_scheduler;
revoke all privileges on budget_reservations from agentforge_scheduler;
revoke all privileges on capacity_reservations from agentforge_scheduler;
drop table budget_reservations;
drop table capacity_reservations;
