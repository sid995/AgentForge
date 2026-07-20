# Threat Model

## Protected assets

- Tenant source and prompts.
- Platform and provider credentials.
- Generated artifacts and images.
- Deployment environments.
- Audit and usage records.
- Kubernetes and cloud control planes.

## Major threats

- Cross-tenant data access.
- Prompt-driven command injection.
- Malicious repository or dependency.
- Container escape and host access.
- Credential theft.
- Unrestricted egress and metadata-service access.
- Supply-chain poisoning.
- Excessive token or infrastructure cost.
- Unauthorized production deployment.

## Security posture

Treat user input, model output, repositories, dependencies, generated code, and agent commands as untrusted. Apply defense in depth at identity, application, database, Kubernetes, network, build, and deployment layers.
