# Database Patterns

## SQLC Workflow

Continua uses SQLC for type-safe database access. Queries are defined in SQL, types are generated.

### Adding a Query

1. **Add to query file**:
   ```sql
   -- db/platform/queries/traces.sql

   -- name: GetTraceWithSpans :many
   SELECT t.*, s.*
   FROM traces t
   LEFT JOIN spans s ON s.trace_id = t.id
   WHERE t.id = $1
   ORDER BY s.started_at;
   ```

2. **Regenerate**:
   ```bash
   make generate
   ```

3. **Use generated method**:
   ```go
   rows, err := s.queries.GetTraceWithSpans(ctx, traceID)
   ```

### Query Annotations

```sql
-- name: GetTrace :one        -- Returns single row (or error)
-- name: ListTraces :many     -- Returns slice
-- name: CreateTrace :one     -- Returns created row
-- name: DeleteTrace :exec    -- Returns only error
-- name: CountTraces :one     -- Returns single value
```

## Schema Design

### Core Tables

```sql
-- Sessions group related traces
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Traces represent agent executions
CREATE TABLE traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES sessions(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'RUNNING',  -- RUNNING, COMPLETED, FAILED
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    total_tokens_in INTEGER DEFAULT 0,
    total_tokens_out INTEGER DEFAULT 0,
    total_cost_usd NUMERIC(10, 6) DEFAULT 0
);

-- Spans are operations within traces (tree structure)
CREATE TABLE spans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
    parent_span_id UUID REFERENCES spans(id),  -- Self-referential for tree
    name TEXT NOT NULL,
    kind TEXT NOT NULL,  -- LLM, TOOL, CHAIN, AGENT, CUSTOM
    status TEXT NOT NULL DEFAULT 'SCHEDULED'
);

-- Payloads store request/response bodies
CREATE TABLE payloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    span_id UUID NOT NULL REFERENCES spans(id) ON DELETE CASCADE,
    direction TEXT NOT NULL,  -- 'request' or 'response'
    body JSONB
);
```

### Relationships

```
Session 1──*─ Trace 1──*─ Span 1──*─ Payload
                          │
                          └── parent_span_id (tree structure)
```

## Migrations

### Creating Migrations

```bash
make migrate-create name=add_api_keys
# Creates: db/platform/migrations/postgres/000X_add_api_keys.up.sql
#          db/platform/migrations/postgres/000X_add_api_keys.down.sql
```

### Migration Rules

1. **Always create both up and down**
2. **Use `IF NOT EXISTS` / `IF EXISTS`** for idempotency
3. **Never modify existing migrations** - create new ones
4. **Test down migrations** - they should fully reverse up

### Example Migration

```sql
-- 0002_add_api_keys.up.sql
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

-- 0002_add_api_keys.down.sql
DROP INDEX IF EXISTS idx_api_keys_hash;
DROP TABLE IF EXISTS api_keys;
```

## Common Query Patterns

### Pagination
```sql
-- name: ListTraces :many
SELECT * FROM traces
ORDER BY started_at DESC
LIMIT $1 OFFSET $2;
```

### Filtering
```sql
-- name: ListTracesBySession :many
SELECT * FROM traces
WHERE session_id = $1
ORDER BY started_at DESC;
```

### Aggregation
```sql
-- name: GetTraceStats :one
SELECT
    COUNT(*) as total_spans,
    SUM(tokens_in) as total_tokens_in,
    SUM(cost_usd) as total_cost
FROM spans
WHERE trace_id = $1;
```

### Tree Reconstruction (CTE)
```sql
-- name: GetSpanTree :many
WITH RECURSIVE span_tree AS (
    SELECT *, 0 as depth
    FROM spans
    WHERE trace_id = $1 AND parent_span_id IS NULL

    UNION ALL

    SELECT s.*, st.depth + 1
    FROM spans s
    JOIN span_tree st ON s.parent_span_id = st.id
)
SELECT * FROM span_tree ORDER BY depth, started_at;
```

## Indexes

Always index:
- Foreign keys (`trace_id`, `session_id`, `parent_span_id`)
- Columns used in WHERE clauses
- Columns used in ORDER BY

```sql
CREATE INDEX idx_spans_trace_id ON spans(trace_id);
CREATE INDEX idx_traces_status ON traces(status);
CREATE INDEX idx_traces_started_at ON traces(started_at);
```
