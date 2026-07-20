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
