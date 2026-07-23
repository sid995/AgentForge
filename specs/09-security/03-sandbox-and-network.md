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

Phase 6.6 treats runner termination output as untrusted. It accepts only a
strict, versioned, 4096-byte result-evidence object, validates the artifact
reference against the CR status contract, and emits only normalized bounded
reasons. Raw termination messages and Kubernetes diagnostic text are not
copied into status, logs, or metric labels.

Phase 6.7 grants the Operator delete access only to namespaced Jobs and Pods.
Graceful cancellation deletes the deterministic owned Job with foreground
propagation. Forced cancellation lists by Job label but acts only after exact
controller APIVersion, kind, name, and UID verification, so a spoofed label
cannot cause another Pod to be deleted.

Phase 6.8 applies a hard platform retry allowlist after the desired CR
allowlist. `POLICY`, `VALIDATION`, `PERMANENT_DEPENDENCY`, and `EXECUTION`
outcomes cannot be relabeled or retried, preventing security-policy,
configuration, and failed-test bypass through retry configuration.

Phase 6.9 never adopts or deletes a retained workspace. It verifies that the
PVC has no controller owner and that its bounded owner-UID and retention
markers match before recording handoff. Ownership conflicts remain visible
until the bounded escalation deadline, and escalation releases only the CR
finalizer; it does not issue destructive storage operations.
