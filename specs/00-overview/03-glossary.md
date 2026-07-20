# Glossary

- **AgentRun**: Durable user-requested unit of agent work.
- **Attempt**: One execution attempt for an AgentRun.
- **Trajectory**: Ordered sequence of agent actions and observations.
- **Control plane**: Services that accept intent and manage desired state.
- **Execution plane**: Sandboxed Kubernetes workloads that run agents and builds.
- **Application plane**: Generated applications deployed for users.
- **Outbox**: Database table holding events to be published reliably.
- **Reconciliation**: Repeated comparison of desired and actual state.
- **Artifact**: Source bundle, test report, logs, SBOM, build output, or similar large result.
- **Release**: Immutable image digest plus deployment configuration.
- **Projection**: Read-optimized view derived from transactional state or events.
