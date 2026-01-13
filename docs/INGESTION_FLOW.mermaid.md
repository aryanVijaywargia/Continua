# Continua Ingestion & Query Flows

> **Version Note**: This document describes the complete ingestion architecture.
> - **v1**: Processes inline (no River queue), uses default project (no auth), no redaction
> - **v1.1+**: Adds River queue for async, authentication, and redaction service
>
> Components marked with 🔮 are planned for v1.1+.

## Main Ingestion Pipeline (Corrected)

```mermaid
sequenceDiagram
    participant App as User App (Python)
    participant SDK as Continua SDK
    participant API as Continua API (Go)
    participant Red as 🔮 Redaction Service (v1.1+)
    participant Val as Validation + Wrapping
    participant Q as 🔮 River Queue (v1.1+)
    participant W as Ingest Worker
    participant PG as PostgreSQL

    Note over App,PG: Ingestion Pipeline - Happy Path

    App->>SDK: trace/span/events recorded
    SDK->>SDK: Batch messages<br/>Merge start/end for same span_id<br/>Drop orphan ends (warn + discard)
    
    SDK->>API: POST /v1/ingest<br/>{batch_key, traces, spans, events}
    
    API->>API: Check size (reject >5MB with 413)
    API->>API: Record server_received_at timestamp
    API->>API: 🔮 Auth: validate API key + scopes (v1.1+, v1 uses default project)

    API->>Red: 🔮 Apply redaction rules (v1.1+, v1 skips)
    Red-->>API: Redacted payload

    API->>Val: Validate + wrap invalid JSON
    Val->>Val: Check JSON validity
    
    alt Invalid JSON
        Val->>Val: Wrap: {"__continua_raw": "...", "__parse_error": "..."}
    end
    
    Val->>Val: Check payload sizes
    
    alt Payload > 64KB threshold
        Val->>Val: Truncate + set metadata<br/>(truncated=true, original_size_bytes, reason)
    end
    
    Val-->>API: Validated payload

    alt Async mode (default)
        API->>Q: 🔮 Enqueue ingest job (v1.1+, v1 processes inline)
        API-->>SDK: 202 Accepted<br/>{status: "accepted", batch_key}
        Q->>W: Dequeue job
    else Sync mode (?sync=true)
        API->>W: Process inline
    end

    W->>PG: BEGIN TRANSACTION
    
    Note over W,PG: STEP 1: Claim batch (FIRST!)
    W->>PG: INSERT INTO ingest_batches<br/>(project_id, batch_key, status='processing')<br/>ON CONFLICT DO NOTHING<br/>RETURNING id
    
    alt Batch already exists (duplicate)
        PG-->>W: No rows returned
        W->>PG: ROLLBACK
        W-->>SDK: 200/202 {status: "duplicate"}
    end
    
    Note over W,PG: STEP 2: Upsert traces + build ID map
    W->>PG: For each trace:<br/>INSERT ... ON CONFLICT DO UPDATE<br/>RETURNING id
    W->>W: Build map: trace_id_text → trace_uuid<br/>(Required: spans.trace_id FK → traces.id)

    Note over W,PG: STEP 3: Upsert spans (using trace UUID map)
    W->>PG: For each span:<br/>trace_uuid = traceMap[span.trace_id]<br/>INSERT ... ON CONFLICT DO UPDATE<br/>COALESCE for patch semantics

    Note over W,PG: STEP 4: Insert events (append-only)
    W->>PG: For each event:<br/>trace_uuid = traceMap[event.trace_id]<br/>INSERT ... ON CONFLICT(idempotency_key) DO NOTHING

    Note over W,PG: STEP 5: Update thinking snapshots
    W->>PG: UPDATE spans SET thinking = ...<br/>WHERE has thinking events

    Note over W,PG: STEP 6: Update batch status
    W->>PG: UPDATE ingest_batches<br/>SET status='accepted', counts...

    W->>PG: COMMIT

    Note over W,PG: STEP 7: Async rollup (separate job)
    W->>Q: Enqueue rollup jobs<br/>for affected traces

    alt Async mode
        Note over SDK: SDK continues, no blocking
    else Sync mode
        W-->>API: Result
        API-->>SDK: 200 OK<br/>{status: "ok", items: [...]}
    end

    Note over App,PG: Error Handling

    rect rgb(255, 230, 230)
        Note over SDK,API: Network failure
        SDK->>SDK: Exponential backoff + retry
        SDK->>API: Retry with same batch_key
        API->>API: Process → duplicate detected
        API-->>SDK: 200/202 {status: "duplicate"}<br/>(NOT 409!)
    end
```

---

## Detailed Worker Flowchart (Corrected)

> **v1 Note**: In v1, auth middleware uses default project, redaction is skipped, and async mode processes inline (no River queue).

```mermaid
flowchart TD
    subgraph SDK["SDK (Python)"]
        A[Trace/Span recorded] --> B[Add to batch queue]
        B --> C{Flush trigger?}
        C -->|Size > 100| D[Flush batch]
        C -->|Bytes > 1MB| D
        C -->|Timer 1s| D
        C -->|Shutdown| D
        D --> E[Merge start/end for same span_id]
        E --> E2{End without start?}
        E2 -->|Yes| E3[Log warning, drop data]
        E2 -->|No| F[POST /v1/ingest]
    end

    subgraph API["API Server (Go)"]
        F --> G0{Size > 5MB?}
        G0 -->|Yes| G0R[413 Request Entity Too Large]
        G0 -->|No| G["🔮 Auth middleware (v1.1+)"]
        G --> H[Parse request]
        H --> I["🔮 Apply redaction rules (v1.1+)"]
        I --> J[Validate + wrap invalid JSON]
        J --> K[Check sizes, truncate]
        K --> L{Sync mode?}
        L -->|Yes| M[Process inline]
        L -->|No| N["🔮 Enqueue to River (v1.1+, v1 inline)"]
        N --> O[Return 202 Accepted]
        M --> P[Return 200 OK]
    end

    subgraph Worker["Ingest Worker"]
        N --> Q[Dequeue job]
        Q --> R[Begin transaction]
        R --> R1[CLAIM: Insert batch idempotency]
        R1 --> R1C{Already exists?}
        R1C -->|Yes| R1D[Rollback, return duplicate]
        R1C -->|No| S[Upsert traces RETURNING id]
        S --> S1[Build trace_id → UUID map]
        S1 --> T[Upsert spans using UUID map]
        T --> U[Insert events using UUID map]
        U --> U1[Update thinking snapshots]
        U1 --> V[Update batch status]
        V --> W[Commit]
        W --> X[Enqueue rollup jobs]
    end

    subgraph DB["PostgreSQL"]
        S --> DB1[(traces)]
        T --> DB2[(spans)]
        U --> DB3[(span_events)]
        R1 --> DB4[(ingest_batches)]
    end

    style SDK fill:#e1f5fe
    style API fill:#fff3e0
    style Worker fill:#f3e5f5
    style DB fill:#e8f5e9
    style E3 fill:#ffcdd2
    style R1D fill:#ffcdd2
    style G0R fill:#ffcdd2
```

---

## Query Flow (with Corrected Endpoints)

```mermaid
sequenceDiagram
    participant UI as Web UI
    participant API as Query API
    participant PG as PostgreSQL

    Note over UI,PG: Trace List (Two-Query Pattern)

    UI->>API: GET /v1/traces?status=error&limit=50
    
    API->>PG: Query 1: Core list (fast)
    Note over API,PG: SELECT id, trace_id, name, status,<br/>start_time, duration_ms, total_cost...<br/>FROM traces WHERE ...<br/>ORDER BY server_received_at DESC<br/>LIMIT 50
    PG-->>API: Trace summaries (no payloads)

    API->>PG: Query 2: Metrics (cacheable)
    Note over API,PG: SELECT COUNT(*), SUM(total_cost),<br/>AVG(duration_ms)<br/>FROM traces WHERE ...
    PG-->>API: Aggregate metrics

    API-->>UI: {traces: [...], metrics: {...}, next_cursor: "..."}

    Note over UI,PG: Trace Detail (includes span_uuid)

    UI->>API: GET /v1/traces/{trace_id}
    
    API->>PG: Trace + span summaries
    Note over API,PG: SELECT t.*, s.id AS span_uuid,<br/>s.span_id, s.name, s.type, s.status...<br/>(no input/output payloads)
    PG-->>API: Trace + spans with span_uuid

    API->>PG: Orphan event count
    PG-->>API: Orphan count

    API-->>UI: {trace, spans (include span_uuid), orphan_count}

    Note over UI,PG: Span Detail (Nested Route)

    UI->>API: GET /v1/traces/{trace_id}/spans/{span_id}
    Note over API: span_id only unique within trace
    API->>PG: Full span with payloads
    PG-->>API: Span with input, output, thinking
    API-->>UI: Full span data

    Note over UI,PG: Alternative: Span by Internal UUID

    UI->>API: GET /v1/spans/{span_uuid}
    Note over API: span_uuid (spans.id) is globally unique
    API->>PG: SELECT * FROM spans WHERE id = $1
    PG-->>API: Full span
    API-->>UI: Full span data

    Note over UI,PG: Events (Nested Route)

    UI->>API: GET /v1/traces/{trace_id}/spans/{span_id}/events
    API->>PG: Events for span
    Note over API,PG: ORDER BY COALESCE(event_ts,<br/>server_ingested_at), sequence
    PG-->>API: Ordered events
    API-->>UI: Events timeline
```

---

## Tree Building (Client-Side)

```mermaid
flowchart LR
    subgraph Input["Flat Span List from API"]
        S1["span_id: A, span_uuid: uuid1<br/>parent: null"]
        S2["span_id: B, span_uuid: uuid2<br/>parent: A"]
        S3["span_id: C, span_uuid: uuid3<br/>parent: A"]
        S4["span_id: D, span_uuid: uuid4<br/>parent: B"]
        S5["span_id: E, span_uuid: uuid5<br/>parent: X"]
    end

    subgraph Process["Tree Building"]
        P1[Create map by span_id]
        P2[For each span]
        P3{Has parent?}
        P4{Parent exists?}
        P5[Add to parent.children]
        P6[Add to roots]
    end

    subgraph Output["Tree Structure"]
        R1["A (root)"]
        R1 --> C1["B"]
        R1 --> C2["C"]
        C1 --> C3["D"]
        R2["E (orphan root)"]
    end

    Input --> Process --> Output

    style S5 fill:#fff3e0
    style R2 fill:#fff3e0
```

Note: Span E has parent X which doesn't exist in the list. 
It becomes an "orphan root" - displayed as a root in the tree.
This is a feature, not a bug (indicates incomplete data).

---

## ER Diagram

```mermaid
erDiagram
    projects ||--o{ api_keys : has
    projects ||--o{ redaction_rules : has
    projects ||--o{ payload_blobs : has
    projects ||--o{ sessions : has
    projects ||--o{ traces : has
    projects ||--o{ ingest_batches : owns
    sessions ||--o{ traces : groups
    traces ||--o{ spans : contains
    traces ||--o{ span_events : has
    projects ||--o{ span_events : owns
    projects ||--o{ scores : owns
    traces ||--o{ scores : annotated
    sessions ||--o{ scores : annotated
    projects ||--o{ external_trace_ids : maps
    traces ||--o{ external_trace_ids : maps
```

---

## HTTP Status Code Reference

| Scenario | Status | Response Body |
|----------|--------|---------------|
| Async, new batch | 202 | `{status: "accepted", batch_key: "..."}` |
| Async, duplicate | 202 | `{status: "duplicate", batch_key: "..."}` |
| Sync, success | 200 | `{status: "ok", items: [...]}` |
| Sync, duplicate | 200 | `{status: "duplicate"}` |
| Validation error | 400 | `{error: "...", details: [...]}` |
| Auth failure | 401 | `{error: "invalid API key"}` |
| Missing scope | 403 | `{error: "insufficient permissions"}` |
| Batch too large | 413 | `{error: "batch exceeds 5MB limit"}` |
| Server error | 500 | `{error: "internal error"}` |

**Important:** Duplicates return success (200/202), NOT 409. A duplicate means "already processed successfully".

---

## Key Implementation Notes

### 1. Batch Idempotency Must Be Claimed First

```sql
-- FIRST operation in transaction
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'processing')
ON CONFLICT (project_id, batch_key) DO NOTHING
RETURNING id;

-- If no rows returned → batch already processed → ROLLBACK immediately
```

### 2. Trace ID Mapping Is Required

```go
// SDK sends external trace_id (TEXT)
// DB uses internal traces.id (UUID)
// spans.trace_id FK → traces.id

traceMap := make(map[string]uuid.UUID)
for _, t := range req.Traces {
    internalID, _ := store.UpsertTrace(ctx, t)
    traceMap[t.TraceID] = internalID
}

// Use internal UUID for spans and events
for _, s := range req.Spans {
    traceUUID := traceMap[s.TraceID]
    store.UpsertSpan(ctx, traceUUID, s)
}
```

### 3. SDK Must Not Create End-Only Spans

```python
def enqueue_span_end(self, trace_id, span_id, **updates):
    key = f"{trace_id}:{span_id}"
    if key in self._spans:
        self._spans[key].merge_end(**updates)
    else:
        # DO NOT create span - required fields missing
        logger.warning(f"Span end without start: {span_id}")
```

### 4. Endpoint Routing for Span Uniqueness

```
# span_id only unique within trace - use nested routes
GET /v1/traces/{trace_id}/spans/{span_id}
GET /v1/traces/{trace_id}/spans/{span_id}/events

# Alternative when internal UUID known
GET /v1/spans/{span_uuid}
```
