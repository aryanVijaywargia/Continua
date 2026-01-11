# Design: Batch Ingestion Pipeline

## Context

Continua is an AI agent observability platform that captures and replays execution traces. The ingestion pipeline is the critical path for data entry from TypeScript/Python SDKs.

## Goals

- Accept batches of up to 5MB with traces, spans, and events
- Provide idempotent ingestion (retry-safe with batch_key)
- Support async processing (non-blocking SDK calls)
- Map external IDs (trace_id TEXT) to internal UUIDs
- Multi-tenant isolation via project_id

## Non-Goals

- Blob storage for large payloads (v1.5+)
- Redaction of sensitive data (placeholder only)
- WebSocket push notifications (separate feature)
- Per-item validation errors (v2)

---

## Key Decisions

### 1. Schema Replacement Strategy

**Decision**: Replace `0001_initial_schema.up.sql` entirely

**Rationale**:
- No production deployments yet
- Cleaner than creating migration 0002
- Avoids migration drift complexity
- User confirmed this approach

### 2. Batch Claiming Pattern

**Decision**: INSERT into `ingest_batches` as FIRST operation in transaction

```sql
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'processing')
ON CONFLICT (project_id, batch_key) DO NOTHING
RETURNING id;
-- If no rows returned → batch already processed → ROLLBACK immediately
```

**Rationale**:
- Prevents duplicate work before expensive upserts
- `ON CONFLICT DO NOTHING` provides atomic claim
- Early rollback saves database resources
- Clean idempotency semantics

### 3. External ID Mapping

**Decision**: Dual ID system with mapping

| Column | Type | Purpose |
|--------|------|---------|
| `id` | UUID | Internal PK, used for FKs |
| `trace_id` | TEXT | External identifier from SDK |
| `span_id` | TEXT | External identifier from SDK |

**Flow**:
1. SDK sends `trace_id: "my-trace-123"`
2. Upsert returns internal UUID
3. Build map: `trace_id` → UUID
4. Use UUID for `spans.trace_id` FK

**Rationale**:
- SDKs use human-readable IDs
- Database uses UUIDs for referential integrity
- Unique constraint on `(project_id, trace_id)` prevents duplicates

### 4. No go-playground/validator

**Decision**: Manual validation functions

**Rationale**:
- Project constraint: no new dependencies except River
- Simple validation rules (required fields, enum values)
- v1: Reject entire batch on any error

### 5. River Queue Integration (v1.1)

**Decision**: Add `github.com/riverqueue/river` for async processing

**Why River**:
- Postgres-backed (no Redis dependency)
- Type-safe job definitions in Go
- Built-in retry and dead-letter queue
- Good Uber Fx integration patterns

**v1 Scope**: Sync mode only (`?sync=true`)
**v1.1 Scope**: Add async mode with River

### 6. Validation Strategy v1

**Decision**: Reject entire batch on validation error

**Response**: HTTP 400 with `{"error": "message"}`

**Rationale**:
- Simpler implementation
- SDKs can fix issues and retry with same batch_key
- v2 can add per-item validation errors

### 7. JSONB Merge Semantics

**Decision**: Different rules for different fields

| Field | Behavior | Rationale |
|-------|----------|-----------|
| `metadata` | Shallow merge (`existing \|\| new`) | Incremental enrichment |
| `input` | Replace if provided | Immutable payload |
| `output` | Replace if provided | Immutable payload |
| `tags` | Replace if non-empty | Array replacement simpler |

### 8. Span Events - No FK to Spans

**Decision**: `span_events.span_id` is TEXT with NO FK

**Rationale**:
- Out-of-order tolerance: events may arrive before spans
- Soft integrity: UI shows "orphan events" count
- Eventual consistency: span may be ingested later

---

## Architecture

### Layer Structure

```
┌─────────────────────────────────────────────┐
│                  API Layer                   │
│  internal/api/ingest.go                     │
│  - HTTP handler                              │
│  - Request validation                        │
│  - Size limit (5MB)                         │
└─────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────┐
│               Service Layer                  │
│  internal/ingest/service.go                 │
│  - Transaction orchestration                 │
│  - Batch claiming (FIRST!)                  │
│  - ID mapping                                │
│  - Payload processing                        │
└─────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────┐
│                Store Layer                   │
│  internal/store/*.go                        │
│  - ClaimBatch                                │
│  - UpsertTrace, GetTraceUUID                │
│  - UpsertSpan                                │
│  - InsertSpanEvents                         │
└─────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────┐
│                 Database                     │
│  PostgreSQL via pgx/v5                      │
│  - projects, ingest_batches                 │
│  - traces, spans, span_events               │
└─────────────────────────────────────────────┘
```

### Transaction Flow

```
1. BEGIN TRANSACTION
2. ClaimBatch(project_id, batch_key)
   └─> If ErrDuplicateBatch: ROLLBACK, return {status: "duplicate"}
3. For each trace:
   └─> UpsertTrace() → returns internal UUID
   └─> Store in traceMap[external_id] = uuid
4. For each span:
   └─> Lookup traceUUID from traceMap (or DB)
   └─> UpsertSpan(traceUUID, span)
5. For each event:
   └─> Lookup traceUUID from traceMap (or DB)
   └─> InsertSpanEvents() (batch insert)
6. UpdateBatchStatus(accepted, counts)
7. COMMIT
8. Return {status: "ok", batch_key: "..."}
```

---

## Schema Design

### New Tables

```sql
-- Multi-tenancy
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Batch idempotency
CREATE TABLE ingest_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    batch_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing',
    server_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_completed_at TIMESTAMPTZ,
    trace_count INT DEFAULT 0,
    span_count INT DEFAULT 0,
    event_count INT DEFAULT 0,
    UNIQUE(project_id, batch_key)
);

-- Append-only events (no FK to spans)
CREATE TABLE span_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    trace_id UUID NOT NULL REFERENCES traces(id),
    span_id TEXT NOT NULL, -- No FK - allows orphans
    event_type TEXT NOT NULL,
    level TEXT NOT NULL DEFAULT 'info',
    event_ts TIMESTAMPTZ,
    sequence INT,
    message TEXT,
    payload JSONB,
    idempotency_key TEXT,
    UNIQUE(project_id, idempotency_key) WHERE idempotency_key IS NOT NULL
);
```

### Modified Tables

```sql
-- traces: add external ID and project FK
ALTER TABLE traces ADD COLUMN trace_id TEXT NOT NULL;
ALTER TABLE traces ADD COLUMN project_id UUID NOT NULL REFERENCES projects(id);
ALTER TABLE traces ADD UNIQUE(project_id, trace_id);

-- spans: add external ID and project FK
ALTER TABLE spans ADD COLUMN span_id TEXT NOT NULL;
ALTER TABLE spans ADD COLUMN project_id UUID NOT NULL REFERENCES projects(id);
ALTER TABLE spans ADD UNIQUE(trace_id, span_id);
```

---

## API Contract

### POST /v1/ingest

```yaml
paths:
  /v1/ingest:
    post:
      operationId: ingest
      parameters:
        - name: sync
          in: query
          schema:
            type: boolean
            default: false
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/IngestRequest'
      responses:
        '200':
          description: Sync success
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/IngestResponse'
        '202':
          description: Async accepted
        '400':
          description: Validation error
        '413':
          description: Payload too large (>5MB)
```

---

## Open Questions (Resolved)

| Question | Resolution |
|----------|------------|
| Schema strategy? | Replace 0001 entirely |
| Multi-tenancy? | Add projects table with FK |
| Default project for v1? | Create default project, add auth in v1.1 |

---

## References

- User's Implementation Guide (provided in prompt)
- Continua Architecture (openspec/project.md)
- Existing OpenAPI contract (contracts/openapi/openapi.yaml)
