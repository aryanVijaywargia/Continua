# Design: Reliability, Search & Sessions

## Context

Phase 3 addresses production-readiness gaps identified after Phase 2 (E2E Usability):

1. **Ingest latency** - Rollups computed inline block the critical path
2. **Data integrity** - Out-of-order span updates lose timestamp information
3. **Discoverability** - No way to find specific traces at scale
4. **Performance** - Indexes missing `project_id` cause slow queries in multi-tenant scenarios
5. **Feature completeness** - Sessions exist in data model but not exposed in UI
6. **SDK robustness** - Python SDK lacks production-grade error handling

### Stakeholders

- **Platform operators**: Need reliable ingest under load
- **Developers**: Need to find specific traces quickly
- **AI engineers**: Need to debug agent sessions holistically

### Constraints

- PostgreSQL 14+ (for generated tsvector columns)
- SQLite migrations are NOT updated for Phase 3 features; local dev/tests must use PostgreSQL or add SQLite-compatible migrations separately
- No new Python dependencies
- Sequential spec execution
- Vendor River migrations (not separate CLI)

## Goals / Non-Goals

### Goals

1. Move rollup computation off the ingest critical path
2. Fix idempotent upserts to handle out-of-order span updates
3. Enable full-text search on traces and span names
4. Ensure project-scoped indexes for queries that already filter by project_id
5. Expose sessions in web UI
6. Make Python SDK production-ready with retries and proper errors

### Non-Goals

- Payload (input/output) search (Phase 4)
- OpenAI/Anthropic monkey-patching in SDK (too invasive)
- Distributed tracing across services
- Real-time search updates via WebSocket
- Session management API (create/update/delete)

## Decisions

### D1: River for Async Job Queue

**Decision**: Use River (github.com/riverqueue/river) for async rollup processing.

**Rationale**:
- PostgreSQL-native (no external dependencies like Redis)
- Transactional job enqueue (job inserted in same transaction as spans)
- Built-in uniqueness constraints (dedupe pending jobs)
- Matches existing tech stack (pgx v5)

**Alternatives Considered**:
- **Simple polling table**: Less features, would need to build deduplication
- **Redis-backed queue**: External dependency, operational overhead
- **Temporal**: Overkill for simple rollup jobs

### D2: Vendor River Migrations

**Decision**: Copy River's required SQL tables into `db/platform/migrations/postgres/`.

**Rationale**:
- Consistent with existing pattern (`continua migrate up`)
- No need for separate River CLI
- Clear upgrade path (copy new migrations when upgrading River)

**Trade-off**: Manual work to update when River schema changes.

### D3: Generated tsvector Columns

**Decision**: Use PostgreSQL generated columns for search vectors (not triggers).

**Rationale**:
- PostgreSQL 14+ guaranteed (user-confirmed minimum version)
- Simpler than trigger-based approach
- Automatic updates when source columns change
- Better performance (computed at storage time)

**Schema**:
```sql
ALTER TABLE traces ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(user_id, '')), 'B')
    ) STORED;
```

### D4: LEAST/GREATEST for Time Merging

**Decision**: Use NULL-safe LEAST/GREATEST in span upsert for time handling.

**Rationale**:
- Current COALESCE-only approach loses timestamps on out-of-order updates
- LEAST ensures earliest start_time preserved
- GREATEST ensures latest end_time preserved
- Triple COALESCE handles all NULL combinations

**SQL Pattern**:
```sql
start_time = COALESCE(
    LEAST(spans.start_time, EXCLUDED.start_time),
    spans.start_time,
    EXCLUDED.start_time
),
end_time = COALESCE(
    GREATEST(spans.end_time, EXCLUDED.end_time),
    spans.end_time,
    EXCLUDED.end_time
)
```

### D5: Project-Scoped Index Strategy

**Decision**: Add project_id-leading indexes for queries that already filter by project_id, while preserving trace_id-leading indexes for queries that don't.

**Rationale**:
- Every list query is scoped to project_id, so leading project_id improves selectivity in multi-tenant workloads
- Composite indexes should only replace trace_id-leading indexes when queries include project_id; otherwise keep existing indexes
- This prevents breaking existing queries that filter by trace_id only

**Implementation**:
- Add new project_id-leading indexes for project-filtered queries
- Preserve trace_id-leading indexes where queries don't include project_id
- Use CONCURRENTLY where possible to avoid locks
- Run ANALYZE after migration

### D6: Manual Retry in Python SDK

**Decision**: Implement retry with exponential backoff manually (no tenacity).

**Rationale**:
- tenacity not in current dependencies
- User requested no new Python deps
- Simple backoff is ~20 lines of code
- Avoids dependency bloat for single use case

**Pattern** (synchronous - SDK uses httpx.Client):
```python
def _send_with_retry(self, batch, max_attempts=3):
    for attempt in range(max_attempts):
        try:
            return self._send_batch(batch)
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            if attempt == max_attempts - 1:
                raise NetworkError(f"Failed after {max_attempts} attempts") from e
            delay = (2 ** attempt) + random.uniform(0, 1)
            time.sleep(delay)
```

### D7: Search Scope

**Decision**: Phase 3 search covers traces + span names only.

**Rationale**:
- Payload search requires different indexing strategy (JSONB or separate tables)
- Traces/spans search covers 90% of use cases
- Payload search explicitly deferred to Phase 4

**Search Fields**:
- Traces: `name`, `user_id`
- Spans: `name` (indexed separately, joined at query time or denormalized later)

## Risks / Trade-offs

### R1: River Version Lock

**Risk**: Vendoring migrations creates tight coupling to River version.

**Mitigation**:
- Document which River version migrations came from
- Check release notes before River upgrades
- Migration diffs are typically small

### R2: Index Recreation Downtime

**Risk**: Dropping and recreating indexes may cause brief query degradation.

**Mitigation**:
- Use `CREATE INDEX CONCURRENTLY` where possible
- Run during low-traffic window
- New indexes created before old ones dropped

### R3: Search Performance at Scale

**Risk**: GIN indexes may degrade with very large datasets (>10M traces).

**Mitigation**:
- Benchmark at 100k traces before merge
- Target: <200ms for search queries
- Add partitioning in Phase 4 if needed

### R4: River Worker Failure

**Risk**: If worker dies, rollups queue up indefinitely.

**Mitigation**:
- Health check includes River worker status
- Job retry with exponential backoff
- Monitor `river_job` table for stuck jobs

## Migration Plan

### Database Migrations (Sequential)

1. **Idempotency fix** (Spec 2): No migration, just SQL query change
2. **River tables** (Spec 1): Add `river_job`, `river_leader`, etc.
3. **Search** (Spec 3): Add `search_vector` column + GIN index
4. **Performance** (Spec 4): Add project_id-leading indexes for project-filtered queries; preserve trace_id-leading indexes

### Rollback Strategy

Each migration has a corresponding down migration:
- River tables: DROP TABLE statements
- Search column: DROP COLUMN
- Indexes: Recreate without `project_id` (restore original)

### Verification Commands

```bash
# After Spec 1 (River)
psql -c "SELECT COUNT(*) FROM river_job"

# After Spec 3 (Search)
EXPLAIN ANALYZE SELECT * FROM traces
WHERE search_vector @@ plainto_tsquery('english', 'test');

# After Spec 4 (Indexes)
\di+ idx_traces_project_*
```

## Open Questions

1. **Span name search strategy**: Should span names be:
   - Denormalized into trace search_vector (simpler query, data duplication)
   - Searched via JOIN (no duplication, complex query)
   - Current decision: JOIN approach, revisit if performance is poor

2. **River job retention**: How long to keep completed jobs?
   - Decision: 7 days default, configurable via env var
   - Enforced via River retention configuration
