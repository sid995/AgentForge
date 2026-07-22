# Scheduler instructions

- PostgreSQL is authoritative; Kafka records are optional wake hints only.
- Never create Kubernetes resources from this process.
- Keep claims short, leases expiring, workers bounded, and final scheduling
  state/reservations/outbox writes transactional.
- Preserve explicit tenant/run predicates after every cross-tenant claim.
- Logs and metrics must not include prompts, source, credentials, raw errors,
  authorization headers, or tenant/run IDs as metric labels.
- Validate changes with unit, PostgreSQL integration, event-contract, race,
  lint, vet, repository, Compose, and container-image gates.
