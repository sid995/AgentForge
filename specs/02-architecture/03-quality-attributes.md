# Quality Attribute Scenarios

## Availability

When one API replica crashes during requests, the load balancer routes to healthy replicas and no accepted command is lost.

## Recoverability

When the scheduler crashes after creating an AgentRun CR but before updating PostgreSQL, reconciliation detects the deterministic CR and repairs state without creating duplicate work.

## Security

When generated code attempts to contact the cloud metadata endpoint, default-deny egress and the egress proxy block the request and emit an audit event.

## Scalability

When queue depth rises, scheduler consumers and execution nodes scale independently without scaling the API unnecessarily.

## Auditability

When a production deployment is approved, the system can show the actor, policy result, Git revision, image digest, timestamp, and resulting Argo CD state.
