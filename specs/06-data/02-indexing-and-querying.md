# Indexing and Querying

## Required indexes

- Queued scheduler index on priority and creation time with a status predicate.
- Tenant-scoped run history index on `(tenant_id, project_id, created_at desc)`.
- Active attempts index on run and state.
- Deployment environment index on `(tenant_id, project_id, environment, created_at desc)`.
- Unpublished outbox index on `published_at is null` and creation time.
- Usage ledger indexes by tenant, project, run, resource type, and observed time.

## Query rules

- All tenant queries require tenant predicate even when RLS is enabled.
- Use keyset pagination for large histories.
- Review `EXPLAIN (ANALYZE, BUFFERS)` for critical queries.
- Avoid unbounded JSONB scans on operational paths.
- Separate analytical queries from state-transition transactions.
