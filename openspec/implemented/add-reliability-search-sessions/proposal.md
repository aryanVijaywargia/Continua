# Change: Add Reliability, Search & Sessions

## Summary

Make Continua production-ready with async processing, full-text search, query performance optimizations, sessions UI, and Python SDK improvements.

## Why

Continua is usable end-to-end after Phase 2 but not production-ready:

1. **Rollups block ingest** - Computing trace aggregates inline adds latency to the critical path
2. **Out-of-order updates break** - Current span upserts don't handle timestamp merging correctly
3. **No search** - Users cannot find traces at scale (>1000 traces = unusable)
4. **Missing indexes** - Queries don't include `project_id` leading to slow multi-tenant queries
5. **Sessions invisible** - Data model exists but no UI to view sessions
6. **SDK rough edges** - Python SDK lacks retries, proper errors, and helper methods

This change makes Continua **reliable and discoverable** for production workloads.

## What Changes

### NEW Capabilities

| Capability | Description |
|------------|-------------|
| **async-rollups** | River-based job queue for trace rollup computation |
| **search** | Full-text search on traces + span names with filtering |
| **sessions-ui** | Sessions list and detail pages in web UI |

### MODIFIED Capabilities

| Capability | Description |
|------------|-------------|
| **idempotency** | Fix LEAST/GREATEST time handling for out-of-order span updates |
| **query-performance** | Add project-scoped indexes where queries already filter by project_id; preserve trace_id-leading indexes |
| **python-sdk-polish** | Custom exceptions, manual retry with backoff, helper methods |

### Key Implementation Notes

1. **River for job queue** - PostgreSQL-native, migrations vendored into `db/platform/migrations/`
2. **PostgreSQL 14+ required** - Use generated tsvector columns for search (no trigger fallback)
3. **Sequential execution** - Specs depend on each other, no parallel tracks
4. **No new Python deps** - Implement retry logic manually (no tenacity)
5. **Search scope** - Traces + span names only; payload search deferred to Phase 4

## Impact

### Affected Specs

| Spec | Type | Description |
|------|------|-------------|
| async-rollups | NEW | River job queue, rollup worker, ingest integration |
| idempotency | MODIFIED | LEAST/GREATEST time merging in span upsert |
| search | NEW | tsvector columns, GIN indexes, filter API |
| query-performance | NEW | Project-scoped indexes for project-filtered queries |
| sessions-ui | NEW | Sessions list, session detail pages |
| python-sdk-polish | MODIFIED | Exceptions, retries, helpers |

### Affected Code

| Path | Change |
|------|--------|
| `go.mod` | ADD: riverqueue deps |
| `db/platform/migrations/postgres/XXXX_*.sql` | ADD: River tables, search indexes, perf indexes |
| `db/platform/queries/spans.sql` | MODIFY: LEAST/GREATEST in UpsertSpan |
| `internal/jobs/` | NEW: River module, rollup worker |
| `internal/ingest/service.go` | MODIFY: Enqueue rollup jobs instead of inline |
| `internal/store/traces.go` | MODIFY: Add filtered query with search |
| `internal/api/server.go` | MODIFY: Add search params to ListTraces |
| `contracts/openapi/openapi.yaml` | MODIFY: Add search/filter query params |
| `web/src/pages/SessionsPage.tsx` | NEW |
| `web/src/pages/SessionDetailPage.tsx` | NEW |
| `web/src/App.tsx` | MODIFY: Add session routes |
| `sdks/python/src/continua/client.py` | MODIFY: Errors, retry |
| `sdks/python/src/continua/trace.py` | MODIFY: Session context |
| `sdks/python/src/continua/span.py` | MODIFY: Helper methods |

### Breaking Changes

None - this adds new functionality and improves existing code.

### Schema Changes (Database)

| Table | Change |
|-------|--------|
| `traces` | ADD: `search_vector` tsvector column |
| `river_*` | ADD: River job queue tables (vendored) |
| Selected tables | MODIFY: Add project_id-leading indexes for project-filtered queries; preserve trace_id-leading indexes |

### Schema Changes (OpenAPI)

| Endpoint | Change |
|----------|--------|
| `GET /api/traces` | ADD: `q`, `status`, `start_time_from`, `start_time_to`, `user_id`, `session_id`, `has_errors`, `min_duration_ms` query params |
| `GET /api/sessions/{id}` | ADD: Single session endpoint (if missing) |

## Dependencies

### Go (to be added)

- `github.com/riverqueue/river` - Job queue
- `github.com/riverqueue/river/riverdriver/riverpgxv5` - River pgx v5 driver

### Web UI (already in package.json)

- No new dependencies

### Python SDK (no new deps)

- Implement retry manually (no tenacity)

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| River complexity | Follow official examples, vendor migrations |
| Search performance | GIN index + EXPLAIN ANALYZE before merge |
| Index recreation downtime | Use CONCURRENTLY where possible |
| Python retry edge cases | Comprehensive unit tests |

## Success Criteria

- [ ] `continua serve` starts with River worker
- [ ] Rollups computed asynchronously (verify via river_job table)
- [ ] Out-of-order span updates preserve correct start/end times
- [ ] Search returns results in <200ms for 100k traces
- [ ] Project-scoped indexes added where queries filter by project_id; trace_id-leading indexes preserved
- [ ] Sessions page shows paginated list
- [ ] Session detail shows related traces
- [ ] Python SDK retries transient errors with backoff
- [ ] `make ci` passes

## Dependency Graph

```
Spec 0 (Discovery)
    |
    v
Spec 2 (Idempotency) ──────────────> Spec 6 (Python SDK Polish)
    |
    v
Spec 1 (Async Rollups - River)
    |
    v
Spec 3 (Search)
    |
    v
Spec 4 (Query Performance)
    |
    v
Spec 5 (Sessions UI)
```

**Critical Path**: `0 → 2 → 1 → 3 → 4 → 5`
**Parallel Track**: `6` (after Spec 2)

Note: Execution is sequential by default as per user request.

## Related Documents

- Discovery: [docs/phase3/spec-0-discovery.md](../../../docs/phase3/spec-0-discovery.md) (to be created)
- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
