# Retention, Backup, and Recovery

## Retention

- Operational run metadata: configurable, default 1 year.
- Full trajectory payloads: 30 to 90 days hot, then archived.
- Audit events: policy-defined, default 7 years for demonstration.
- Build images: retain referenced releases; expire unreferenced images.
- Temporary workspaces: delete after terminal completion and grace period.
- Published outbox events: 7 days; unpublished, claimed, or repair-required
  rows are never automatically deleted.
- Processed-event markers: at least the maximum replay window, default 1 year.
- Kafka lifecycle topics: 7 to 30 days by stream; retry topics default 14 days
  and DLQ topics default 90 days.

## PostgreSQL

- Automated snapshots and point-in-time recovery.
- Restore tests performed regularly.
- Schema migrations included in recovery exercises.

## Object storage

- Versioning for critical artifacts.
- Lifecycle transitions for archives.
- Integrity checksum stored with every artifact reference.

## Recovery targets

Initial target RPO 5 minutes and RTO 60 minutes for control-plane state.
