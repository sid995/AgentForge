# Non-Functional Requirements

## Reliability

- API availability target: 99.9%.
- Accepted commands must survive process restart.
- Consumers must safely process duplicate events.
- Controllers must recover by observing current state rather than relying on memory.

## Performance

- Normal API read P95 below 300 ms.
- Run acceptance P95 below 500 ms.
- Dashboard status freshness below 5 seconds.
- Cancellation acknowledgment below 5 seconds.

## Security

- Least-privilege service identities.
- Short-lived credentials for workloads.
- No privileged containers, host paths, host networking, or Docker socket mounts.
- Default-deny ingress and egress for execution workloads.

## Operability

- Structured logs, metrics, traces, alerts, dashboards, and runbooks for every service.
- Every incident exercise produces corrective actions.

## Maintainability

- Domain logic isolated from transports and infrastructure using ports and adapters.
- Public contracts versioned and backward-compatible during migrations.
