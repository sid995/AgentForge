# Sandbox and Network Security

## Pod controls

- run as non-root.
- read-only root filesystem.
- drop all capabilities.
- disallow privilege escalation.
- RuntimeDefault seccomp or stronger sandbox runtime.
- no host namespaces, host paths, device access, or Docker socket.
- process, file, disk, CPU, memory, and execution limits.

## Network

- Default-deny ingress and egress.
- Egress through controlled proxy where practical.
- Block link-local and metadata-service ranges.
- Domain allowlists are resolved carefully and protected from DNS rebinding.
- mTLS for service-to-service traffic when adopted.

## Build security

Rootless builds, pinned builder digest, trusted base images, SBOM, vulnerability scan, provenance, and image signing.

## Phase 6.4 enforced workload policy

The Operator builders and defense-in-depth validator require UID/GID 65532,
`runAsNonRoot`, `RuntimeDefault` seccomp, read-only root filesystem, no added
capabilities, `ALL` capabilities dropped, privilege disabled, and privilege
escalation disabled. Writable space is limited to the workspace and bounded
`emptyDir` mounts for `/tmp` and `/home/agent`.

Host networking, PID/IPC namespaces, `hostPath`, Docker socket mounts,
projected Kubernetes API tokens, init containers, and ephemeral containers are
rejected. ServiceAccount and Pod token automount are both false. Mutation tests
prove each prohibited setting is detected. Network-policy tests prove deny-all
isolation and reject wildcard, overly broad, link-local, and metadata-service
egress.
