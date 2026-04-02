> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 4: Query Performance

## Summary

Added project-scoped indexes to improve multi-tenant query performance. The original indexes on `traces` table were not optimized for queries filtered by `project_id`, causing inefficient scans in multi-tenant environments.

## Changes Made

### 1. Created Index Migration

**File**: `db/platform/migrations/postgres/000005_fix_index_scoping.up.sql`

Replaced non-scoped indexes with project-scoped versions:

```sql
-- Drop non-project-scoped indexes
DROP INDEX IF EXISTS idx_traces_started_at;
DROP INDEX IF EXISTS idx_traces_server_received;

-- Create project-scoped indexes
CREATE INDEX idx_traces_project_started_at
    ON traces(project_id, start_time DESC NULLS LAST);

CREATE INDEX idx_traces_project_server_received
    ON traces(project_id, server_received_at DESC);

ANALYZE traces;
```

### 2. Index Design Rationale

**Before**: Indexes on `start_time` and `server_received_at` alone
- Query planner could not efficiently filter by project first
- Full index scans required even for single-project queries

**After**: Composite indexes with `project_id` as leading column
- B-tree can skip directly to project's data
- Time-based ordering preserved within each project partition
- Optimal for `WHERE project_id = $1 ORDER BY start_time DESC` pattern

### 3. Why These Specific Indexes

The search implementation (`internal/store/search.go`) and list queries use:

```sql
SELECT ... FROM traces
WHERE project_id = $1
ORDER BY COALESCE(start_time, server_received_at) DESC
```

Both indexes support this access pattern:
- `idx_traces_project_started_at`: Optimizes queries with `start_time` populated
- `idx_traces_project_server_received`: Fallback for server-received ordering

### 4. Existing Project-Scoped Indexes

These already existed and remain unchanged:
- `idx_traces_session_started` - `(session_id, start_time DESC NULLS LAST)`
- `idx_spans_trace` - `(trace_id)`
- `idx_spans_parent` - `(parent_span_id)`

## Performance Impact

| Query Pattern | Before | After |
|--------------|--------|-------|
| List traces by project | Index scan + filter | Index range scan |
| Search with project filter | Seq scan likely | Index range scan |
| Pagination (OFFSET/LIMIT) | Full scan to offset | Efficient skip |

## Rollback

**File**: `db/platform/migrations/postgres/000005_fix_index_scoping.down.sql`

```sql
DROP INDEX IF EXISTS idx_traces_project_started_at;
DROP INDEX IF EXISTS idx_traces_project_server_received;

CREATE INDEX idx_traces_started_at ON traces(start_time DESC NULLS LAST);
CREATE INDEX idx_traces_server_received ON traces(server_received_at DESC);

ANALYZE traces;
```

## Verification

```bash
# Apply migration
make migrate

# Verify indexes exist
psql -c "SELECT indexname FROM pg_indexes WHERE tablename = 'traces';"

# Check query plan uses new indexes
psql -c "EXPLAIN ANALYZE SELECT * FROM traces WHERE project_id = '...' ORDER BY start_time DESC LIMIT 50;"
```

---

=== SPEC 4 COMPLETE: READY FOR REVIEW ===
