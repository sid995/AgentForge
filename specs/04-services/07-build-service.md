# Build Service

## Responsibilities

- Consume accepted source artifacts.
- Create isolated rootless build Jobs.
- Build OCI image with pinned builder version.
- Generate SBOM and provenance.
- Scan dependencies and image.
- Sign accepted image.
- Push and record immutable digest.

## Idempotency

Build identity is derived from source digest, build specification digest, and builder version. Repeated requests return the existing accepted build when policy permits.

## Security

- Never mount the host Docker socket.
- Restrict registry and package egress.
- Use trusted base-image allowlists.
- Enforce image size, build duration, and vulnerability policy.
