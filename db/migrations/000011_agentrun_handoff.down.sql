revoke select, insert on processed_events from agentforge_handoff;
revoke select on agentrun_handoff_intents from agentforge_handoff;
revoke select on clusters, cluster_tenant_allowlist from agentforge_handoff;
revoke usage on schema public from agentforge_handoff;
revoke select, insert on agentrun_handoff_intents from agentforge_scheduler;
drop table agentrun_handoff_intents;
revoke select (prompt_reference, max_attempts) on agent_runs from agentforge_scheduler;
