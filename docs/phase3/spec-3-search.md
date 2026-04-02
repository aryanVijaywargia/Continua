> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 3: Search & Filtering

## Summary

Added full-text search and filtering capabilities for traces, enabling users to find traces at scale with rich query options.

## Changes Made

### 1. Updated OpenAPI Spec

**File**: `contracts/openapi/openapi.yaml`

Added query parameters to `GET /api/traces`:
- `q` (string): Full-text search query (searches trace name and user_id)
- `status` (string enum): Filter by trace status (running, completed, failed)
- `start_time_from` (datetime): Filter traces starting at or after this time
- `start_time_to` (datetime): Filter traces starting at or before this time
- `user_id` (string): Filter by user ID
- `has_errors` (boolean): Filter traces with errors (error_count > 0)
- `min_duration_ms` (integer): Filter traces with duration >= this value

Added new endpoint:
- `GET /api/sessions/{id}` - Get a session by ID

Updated Session schema with:
- `user_id` (string): User identifier for this session
- `trace_count` (integer): Number of traces in this session

### 2. Created Search Migration

**File**: `db/platform/migrations/postgres/000004_add_search.up.sql`

Added full-text search infrastructure:
- `search_vector` tsvector column on traces table (updated via trigger)
- `search_vector` tsvector column on spans table (updated via trigger)
- GIN indexes for fast full-text search
- Triggers to automatically update search vectors on INSERT/UPDATE
- Backfill existing rows

Key features:
- Trace search vector: weighted combination of name (A) and user_id (B)
- Span search vector: span name (A)
- Uses `plainto_tsquery` for AND semantics (all terms must match)

### 3. Fixed Migration File Numbering

Renamed initial migration for consistent sorting:
- `0001_initial_schema.up.sql` → `000001_initial_schema.up.sql`
- `0001_initial_schema.down.sql` → `000001_initial_schema.down.sql`

This ensures SQLC processes migrations in correct order.

### 4. Implemented Store Search Method

**File**: `internal/store/search.go`

New types:
```go
type TraceFilter struct {
    ProjectID     uuid.UUID
    Query         string     // Full-text search query
    Status        string     // running, completed, failed
    StartTimeFrom *time.Time
    StartTimeTo   *time.Time
    UserID        string
    SessionID     *uuid.UUID
    HasErrors     *bool
    MinDurationMs *int64
    Limit         int32
    Offset        int32
}

type TraceSearchResult struct {
    Traces []platform.Trace
    Total  int64
}
```

New method:
- `ListTracesFiltered(ctx, filter) (TraceSearchResult, error)` - Dynamic query building with parameterized SQL

Features:
- Full-text search using tsvector/GIN indexes
- Case-insensitive status mapping (FAILED matches 'failed', 'error', 'cancelled')
- Time range filtering with COALESCE(start_time, server_received_at)
- Duration filtering with running trace support (uses now() for end_time)
- Pagination with accurate total count

### 5. Updated API Handler

**File**: `internal/api/server.go`

- Updated `ListTraces` to use new filter parameters
- Added `GetSession` method for session detail endpoint
- Detects when filters are provided and uses `ListTracesFiltered`
- Falls back to simple `ListTraces` when no filters provided

### 6. Updated Mapper

**File**: `internal/api/mapper.go`

- Updated `sessionToAPI` to include `user_id` field

## Search Semantics

### Full-Text Search
- Uses PostgreSQL `plainto_tsquery('english', q)` for parsing
- AND semantics: all terms must match (not OR)
- Searches trace name (weight A) and user_id (weight B)
- Empty or whitespace-only query skips FTS filter

### Status Mapping
| API Status | DB Status Values |
|-----------|------------------|
| running | running |
| completed | completed |
| failed | failed, error, cancelled |

### Time Range
- Uses `COALESCE(start_time, server_received_at)` for filtering
- Handles traces without explicit start_time

### Duration Calculation
```sql
EXTRACT(EPOCH FROM (
    COALESCE(end_time, now()) -
    COALESCE(start_time, server_received_at)
)) * 1000 >= min_duration_ms
```

## Test Coverage

Tests exist in `internal/store/search_test.go`:
- `TestSearch_ByTraceName` - Full-text search with AND semantics
- `TestSearch_ByUserID` - Search by user_id
- `TestSearch_EmptyQuery` - Empty query returns all traces
- `TestSearch_FilterByStatus` - Status filtering
- `TestSearch_FilterByStatusFailed` - FAILED status mapping
- `TestSearch_FilterByTimeRange` - Time range filtering
- `TestSearch_FilterByTimeRangeWithNullStartTime` - Fallback to server_received_at
- `TestSearch_FilterByHasErrors` - Error filtering
- `TestSearch_FilterByMinDuration` - Duration filtering
- `TestSearch_FindTraceBySpanName` - Search by span name
- `TestSearch_MultipleSpansMatchSameTrace` - Deduplication
- `TestSearch_CombinedSearchAndFilter` - Combined search + filters
- `TestSearch_Pagination` - Pagination semantics

## Verification

```bash
# Build
go build ./cmd/continua/...

# Run migrations (after starting PostgreSQL)
make migrate

# Regenerate code
make generate

# Test search functionality
curl "http://localhost:8080/api/traces?q=checkout&status=completed"
```

---

=== SPEC 3 COMPLETE: READY FOR REVIEW ===
