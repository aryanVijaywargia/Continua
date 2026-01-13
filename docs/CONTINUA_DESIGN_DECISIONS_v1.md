# Continua v1 Schema & Design Decisions

> **Summary of changes from initial architecture to final v1 design**

---

## Overview

This document captures all decisions made during the architecture review process, including what changed from the initial design and why.

---

## Design Decisions Summary

### 1. Payload Handling: Wrapper Approach

**Decision:** Store all payloads as valid JSONB by wrapping invalid JSON

**Initial Approach:** JSONB columns that would reject invalid JSON

**Final Approach:**
```json
// Valid JSON stored as-is
{"messages": [{"role": "user", "content": "Hello"}]}

// Invalid JSON wrapped at ingestion
{
  "__continua_raw": "malformed{json here",
  "__parse_error": "unexpected EOF at position 15",
  "__content_type": "text/plain"
}
```

**Rationale:**
- Never-fail ingestion (debugger must show what was actually sent)
- Simpler queries than dual-column approach (always query JSONB)
- UI can detect wrapped payloads by checking for `__continua_raw` key
- Avoids column explosion (no `*_json` + `*_raw` + `*_is_json` per field)

**Alternative Rejected:** Dual columns (`input_json JSONB` + `input_raw TEXT`) - too verbose

---

### 2. Span Events: No Foreign Key

**Decision:** `span_events.span_id` is TEXT with no FK to `spans`

**Initial Approach:** UUID FK to spans(id) or composite FK

**Final Approach:**
```sql
CREATE TABLE span_events (
    ...
    span_id TEXT NOT NULL,  -- No FK
    ...
);

-- Soft integrity via index
CREATE INDEX idx_span_events_span 
    ON span_events(project_id, trace_id, span_id);
```

**Rationale:**
- Out-of-order ingestion is common (events arrive before spans)
- Orphan events are useful debugging signal ("SDK crashed mid-span")
- Avoids "skeleton span" garbage data from upserts
- Simpler ingestion logic (no conditional span creation)

**Query Pattern for Orphans:**
```sql
SELECT e.* FROM span_events e
LEFT JOIN spans s ON s.trace_id = e.trace_id AND s.span_id = e.span_id
WHERE e.trace_id = $1 AND s.id IS NULL;
```

---

### 3. Span Events: Level Column

**Decision:** Add `level` column with CHECK constraint

**Initial Approach:** Missing from original schema

**Final Approach:**
```sql
level TEXT NOT NULL DEFAULT 'info'
    CHECK (level IN ('debug', 'info', 'warn', 'error'))
```

**Rationale:**
- Essential for timeline filtering and coloring in UI
- Stable vocabulary (unlike event_type which evolves)
- Key differentiator for debugging workflows

---

### 4. Event Type: No Database Constraint

**Decision:** `event_type TEXT` validated at application layer only

**Initial Approach:** CHECK constraint with fixed list

**Final Approach:**
```sql
event_type TEXT NOT NULL,  -- No CHECK constraint
```

**Rationale:**
- Event types evolve rapidly (new integrations, new patterns)
- Adding new types shouldn't require schema migration
- Application layer validation provides same protection
- Follows "liberal in what you accept" principle for ingestion

---

### 5. Server-Trust Timestamps

**Decision:** Single `server_received_at` / `server_ingested_at` alongside client times

**Initial Approach:** Either no server timestamps or full duplication

**Final Approach:**
```sql
-- On traces and spans
server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

-- On span_events
server_ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

-- Client times preserved
start_time TIMESTAMPTZ,  -- from SDK
end_time TIMESTAMPTZ,    -- from SDK
event_ts TIMESTAMPTZ,    -- from SDK
```

**Rationale:**
- Client clocks are unreliable (skew, drift, wrong timezone)
- Server timestamp provides reliable ordering fallback
- Don't need full duplication (no `server_start_time`, `server_end_time`)
- UI defaults to client time for display, falls back to server time

---

### 6. Thinking Capture

**Decision:** Primary via span_events + denormalized snapshot column

**Initial Approach:** Not explicitly addressed

**Final Approach:**
```sql
-- In spans table
thinking TEXT,  -- Denormalized snapshot of latest thinking
thinking_truncated BOOLEAN DEFAULT FALSE,
thinking_original_size_bytes BIGINT,
thinking_truncation_reason TEXT,
thinking_blob_id UUID REFERENCES payload_blobs(id),

-- In span_events (streaming/full history)
-- event_type = 'llm.thinking' or 'llm.thinking_chunk'
```

**Rationale:**
- Events support streaming (thinking arrives in chunks)
- Snapshot enables fast UI queries without stitching chunks
- Avoids schema sprawl (only thinking gets this treatment)
- Worker updates snapshot from events asynchronously

---

### 7. Truncation Metadata

**Decision:** Full metadata for each truncatable field

**Initial Approach:** Boolean flags only

**Final Approach:**
```sql
-- Per payload field
input_truncated BOOLEAN DEFAULT FALSE,
input_original_size_bytes BIGINT,
input_truncation_reason TEXT,
input_blob_id UUID REFERENCES payload_blobs(id),
```

**Rationale:**
- UI needs to show "truncated from X bytes"
- Reason helps debugging ("size_limit", "depth_limit", etc.)
- Blob ID enables "fetch full payload" workflow

---

### 8. Blob Reference Columns

**Decision:** Add nullable blob ref columns now (for v1.5 readiness)

**Initial Approach:** Add columns later when implementing blob storage

**Final Approach:**
```sql
-- In spans
input_blob_id UUID REFERENCES payload_blobs(id),
output_blob_id UUID REFERENCES payload_blobs(id),
thinking_blob_id UUID REFERENCES payload_blobs(id),

-- In span_events
payload_blob_id UUID REFERENCES payload_blobs(id),
```

**Rationale:**
- Avoids painful migration later
- Columns can remain NULL in v1
- Schema is blob-ready when feature implemented
- Minimal storage cost for unused columns

---

### 9. ID Terminology

**Decision:** Keep `trace_id`, `span_id` terminology

**Alternative Rejected:** Renaming to `trace_key`, `span_key`

**Rationale:**
- OTel uses `trace_id`, `span_id` - ecosystem alignment
- All reference implementations use these terms
- SDK developers expect these names
- Less cognitive load, easier onboarding

---

### 10. Span Type Default

**Decision:** Default to `'custom'` instead of `'span'`

**Rationale:**
- Signals "you should specify a type"
- Distinguishes intentional generic spans from unclassified
- Minor point, either works

---

### 11. Keep scores Table

**Decision:** Include `scores` table in v1 schema

**Alternative Considered:** Defer to later phase

**Rationale:**
- Core feature identified in Langfuse analysis
- Minimal additional complexity (~30 lines SQL)
- Annotations/evaluations are table stakes for observability
- Avoids migration when building scores API

---

### 12. Keep external_trace_ids Table

**Decision:** Include OTel ID mapping table in v1 schema

**Alternative Considered:** Defer to v1.5 with OTel support

**Rationale:**
- Trivial cost (~15 lines SQL)
- Enables OTel support without migration
- Can remain empty until needed
- Clean separation of external vs internal IDs

---

## Additional Decisions from Review

These decisions were added after architecture review feedback.

---

### 13. Endpoint Routing: Nested Routes for Span Uniqueness

**Decision:** Use nested routes since span_id is only unique within a trace

**Issue:** `GET /v1/spans/{span_id}` is ambiguous because span_id uniqueness is `(trace_id, span_id)`

**Final Routes:**
```
# Primary (uses external IDs)
GET /v1/traces/{trace_id}/spans/{span_id}
GET /v1/traces/{trace_id}/spans/{span_id}/events

# Alternative (uses internal UUID - useful for UI after initial load)
GET /v1/spans/{span_uuid}
```

**Rationale:**
- External span_id is only unique within its trace
- Internal span_uuid (spans.id) is globally unique
- Trace detail response includes both: `{span_id: "sp_001", span_uuid: "uuid-here"}`
- UI can use internal UUID for subsequent lookups

---

### 14. Trace ID Mapping in Worker

**Decision:** Worker must map external trace_id (TEXT) → internal traces.id (UUID)

**Why Required:**
- SDK sends `trace_id` as TEXT (external identifier)
- Database stores `traces.id` as UUID (internal PK)
- `spans.trace_id` FK references `traces.id` (UUID)
- `span_events.trace_id` FK references `traces.id` (UUID)

**Implementation:**
```go
// Build map during trace upsert
traceMap := make(map[string]uuid.UUID)
for _, t := range req.Traces {
    internalID, _ := store.UpsertTrace(ctx, t)
    traceMap[t.TraceID] = internalID  // external → internal
}

// Use map for spans/events
for _, s := range req.Spans {
    traceUUID := traceMap[s.TraceID]
    store.UpsertSpan(ctx, traceUUID, s)
}
```

---

### 15. Batch Idempotency: Claim First

**Decision:** Claim batch idempotency at start of transaction, not end

**Correct Transaction Order:**
1. `INSERT ingest_batches ON CONFLICT DO NOTHING RETURNING id`
2. If no rows returned → batch already processed → return success
3. Upsert traces (RETURNING id for mapping)
4. Upsert spans
5. Insert events
6. Update batch status
7. COMMIT

**Rationale:**
- Prevents double-processing
- Duplicate batches return immediately without redoing work
- Batch status updated only on success

---

### 16. HTTP Status Codes: No 409 for Duplicates

**Decision:** Duplicate batches return 200/202, not 409

| Scenario | Status |
|----------|--------|
| Async, new batch | 202 |
| Async, duplicate | 202 |
| Sync, success | 200 |
| Sync, duplicate | 200 |

**Rationale:**
- 409 Conflict implies "you need to change something and retry"
- Duplicate batch means "already processed successfully"
- From client's perspective, duplicate = success
- Simplifies client retry logic

---

### 17. Batch Size Limit

**Decision:** Reject batches > 5MB at API layer (413 error)

**Rationale:**
- Prevents job queue payload bloat
- 5MB is plenty for typical batches (100s of spans)
- Simple to implement (http.MaxBytesReader)
- Blob-based job storage can be added in v1.5 if needed

---

### 18. SDK: No End-Only Spans

**Decision:** SDK drops span end data if no corresponding start exists

**Problem:** Creating spans from end-only data would require:
- Placeholder name → garbage data
- Guessed start_time → incorrect

**Solution:**
```python
def enqueue_span_end(self, trace_id, span_id, **updates):
    if key in self._spans:
        self._spans[key].merge_end(**updates)
    else:
        logger.warning(f"Span end without start: {span_id}")
        # DO NOT create partial span
```

**Rationale:**
- DB requires `name NOT NULL` and `start_time NOT NULL`
- Debugger should show real data, not placeholders
- Warning helps developers fix instrumentation

---

### 19. Timestamp Semantics Clarification

**Decision:** `server_received_at` = time of database write (includes queue delay)

**Not Changed:** We're not passing API receive time through job payload for v1

**Documented Behavior:**
- `server_received_at` on traces/spans = when row written to DB
- `server_ingested_at` on events = when row written to DB
- Primary purpose = clock-skew protection (server-controlled vs client-controlled)
- Typical queue delay < 100ms in normal operation
- Can add `api_received_at` column in v1.5 if precise timing needed

---

## Schema Changes Summary

### Tables Added
- None (all tables were in original plan)

### Columns Added to `spans`
- `thinking TEXT`
- `thinking_truncated BOOLEAN`
- `thinking_original_size_bytes BIGINT`
- `thinking_truncation_reason TEXT`
- `thinking_blob_id UUID`
- `input_original_size_bytes BIGINT`
- `input_truncation_reason TEXT`
- `input_blob_id UUID`
- `output_original_size_bytes BIGINT`
- `output_truncation_reason TEXT`
- `output_blob_id UUID`
- `server_received_at TIMESTAMPTZ`

### Columns Added to `span_events`
- `level TEXT` (with CHECK constraint)
- `original_size_bytes BIGINT`
- `truncation_reason TEXT`
- `payload_blob_id UUID`
- `server_ingested_at TIMESTAMPTZ`
- `project_id UUID` (denormalized for efficient queries)

### Columns Added to `traces`
- `server_received_at TIMESTAMPTZ`

### Columns Added to `sessions`
- `server_received_at TIMESTAMPTZ`

### Constraints Changed
- `span_events.span_id`: Changed from UUID FK to TEXT (no FK)
- `span_events.event_type`: Removed CHECK constraint

### Constraints Added
- `span_events.level`: Added CHECK constraint

---

## Files Produced

| File | Description |
|------|-------------|
| `migrations/001_initial_schema.sql` | Production-ready PostgreSQL schema |
| `docs/CONTINUA_ARCHITECTURE_PLAN_v1.md` | Complete architecture specification |
| `docs/CONTINUA_IMPLEMENTATION_CHECKLIST_v1.md` | Task-level implementation guide |
| `docs/CONTINUA_DESIGN_DECISIONS_v1.md` | This document |

---

## Implementation Priority

1. **Schema** - Run migrations first
2. **Store Layer** - Implement CRUD operations
3. **Ingest Service** - JSON wrapping, truncation, writes
4. **API Endpoints** - REST handlers
5. **SDK** - Python client with batching
6. **UI** - React frontend

---

*Document generated: 2025-01-08*
