# Spec 0: Discovery Report

## Summary

This document captures the current state of the codebase before implementing Phase 3 changes (Reliability, Search & Sessions).

## 0.1 River / Job Queue Status

**Finding**: No existing job queue in codebase.

- **go.mod**: No riverqueue deps present
- **Current rollup location**: `internal/ingest/service.go` lines 143-150
- **Current behavior**: Rollups are computed inline during ingest via `ComputeAndUpdateTraceRollupsTx()`

```go
// From internal/ingest/service.go:143-150
for _, traceUUID := range traceMap {
    if err := s.store.ComputeAndUpdateTraceRollupsTx(ctx, tx, traceUUID); err != nil {
        log.Printf("Warning: failed to compute rollups for trace %s: %v", traceUUID, err)
        // Continue - rollups are eventually consistent
    }
}
```

**Test scaffolding exists**: `internal/jobs/rollup_test.go` contains tests expecting River implementation but the `internal/jobs/` module implementation is missing.

## 0.2 Idempotency Implementation

**Finding**: Current UpsertSpan uses COALESCE-only approach.

**File**: `db/platform/queries/spans.sql`

**Current SQL (lines 72-73)**:
```sql
start_time = COALESCE(EXCLUDED.start_time, spans.start_time),
end_time = COALESCE(EXCLUDED.end_time, spans.end_time),
```

**Issue**: This approach loses timestamps on out-of-order updates. If end_time arrives first and then start_time arrives later, the current logic works. But if:
1. Span created with start_time only
2. Later, an update with end_time (but no start_time) arrives
3. Then another update with earlier start_time arrives

The COALESCE will take the first non-null value, not necessarily the earliest start_time.

**Required change**: Use LEAST/GREATEST pattern as specified in tasks.md

## 0.3 Search Capabilities

**Finding**: No search capabilities exist.

- **tsvector columns**: None
- **GIN indexes**: None
- **OpenAPI search params**: Only `limit`, `offset`, `session_id` on `GET /api/traces`

## 0.4 Index Audit

### Traces Table Indexes

| Index Name | Columns | Has project_id? |
|------------|---------|-----------------|
| idx_traces_project_trace | project_id, trace_id | ✅ Yes |
| idx_traces_project_session | project_id, session_id | ✅ Yes |
| idx_traces_status | project_id, status | ✅ Yes |
| idx_traces_started_at | start_time DESC NULLS LAST | ❌ No |
| idx_traces_server_received | server_received_at DESC | ❌ No |

### Spans Table Indexes

| Index Name | Columns | Notes |
|------------|---------|-------|
| idx_spans_trace | trace_id | For trace lookups |
| idx_spans_trace_span | trace_id, span_id | Uniqueness |
| idx_spans_project | project_id | Project filtering |
| idx_spans_type | project_id, type | Has project_id |
| idx_spans_start_time | start_time | No project_id |
| idx_spans_parent | trace_id, parent_span_id | Hierarchy |

### Indexes Needing Project Scope

Per Spec 4, need to add project_id to these traces indexes (for queries that filter by project_id):
- `idx_traces_started_at` → `idx_traces_project_started_at`
- `idx_traces_server_received` → `idx_traces_project_server_received`

## 0.5 Sessions API Completeness

### OpenAPI Status

| Endpoint | Status | Notes |
|----------|--------|-------|
| `GET /api/sessions` | ✅ Exists | Has limit/offset params |
| `GET /api/sessions/{id}` | ❌ Missing | Needs to be added |

### Session Schema
Current schema has: `id`, `name`, `metadata`, `created_at`
Missing: `user_id`, `trace_count`

### Store Methods
- `GetSession` ✅ Exists
- `ListSessions` ✅ Exists
- `CountSessions` ✅ Exists
- `CreateSession` ✅ Exists
- Session with trace_count ❌ Missing (needs JOIN)

## 0.6 Python SDK Structure

**File**: `sdks/python/src/continua/client.py`

### Current State

| Feature | Status | Notes |
|---------|--------|-------|
| Custom exceptions | ❌ Missing | Uses generic `httpx.HTTPError` |
| Retry logic | ❌ Missing | Single attempt only |
| Session context | ❌ Missing | No `session()` context manager |
| Span helpers | ❌ Missing | No `set_llm_response()`, etc. |

### Current Error Handling (lines 204-209)
```python
try:
    response = self._client.post("/v1/ingest", json=payload)
    response.raise_for_status()
except httpx.HTTPError as e:
    # Log but don't raise - we don't want to crash the application
    print(f"Continua: Failed to send batch: {e}")
```

**Issues**:
1. Swallows all errors with print statement
2. No distinction between auth errors (401) and transient errors
3. No retry for transient failures

### Test Scaffolding
Test file exists: `sdks/python/tests/test_errors.py` expecting:
- `continua.exceptions.AuthenticationError`
- `continua.exceptions.RateLimitError`
- `continua.exceptions.ValidationError`
- `continua.exceptions.NetworkError`
- `continua.session()` context manager
- `span.set_llm_response()`, `span.set_tool_call()`, `span.log()`

## Implementation Order

Based on dependencies in proposal.md:

```
Spec 0 (Discovery) ✅ COMPLETE
    |
    v
Spec 2 (Idempotency) ← NEXT
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

Spec 6 (Python SDK) - Can run after Spec 2 (parallel track)
```

---

=== SPEC 0 COMPLETE ===
