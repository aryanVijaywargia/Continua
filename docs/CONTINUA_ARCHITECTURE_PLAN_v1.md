> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](./README.md) and [DEBUGGER_PLATFORM_BASELINE.md](./DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Continua Architecture Plan v1.0

> **AI Agent Observability & Debugging Platform**
> 
> *"Where exactly did my agent go wrong, and what data caused it?"*

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Core Architecture Decisions](#2-core-architecture-decisions)
3. [Data Model](#3-data-model)
4. [System Components & Flows](#4-system-components--flows)
5. [API Design](#5-api-design)
6. [SDK Design](#6-sdk-design)
7. [UI Architecture](#7-ui-architecture)
8. [Durability & Replay Foundation](#8-durability--replay-foundation)
9. [Implementation Phases](#9-implementation-phases)
10. [Appendix: Key Technical Decisions](#appendix-key-technical-decisions)

---

## 1. Executive Summary

### Vision

Continua is an observability and debugging platform for AI agents that enables developers to understand exactly where agent executions went wrong and what data caused failures. The platform provides deterministic replay, step-through debugging, and time-travel capabilities for complex multi-agent workflows.

### Core Differentiators

| Feature | Continua Approach | Competitors |
|---------|-------------------|-------------|
| **Event Log** | First-class append-only `span_events` | Embedded in metadata or separate observations |
| **Payload Handling** | Never-fail (wrapper for invalid JSON) | Reject or truncate silently |
| **Span Integrity** | Soft integrity (no FK) for out-of-order tolerance | Strict FK or no integrity |
| **Thinking Capture** | Events + snapshot column | Not first-class |
| **Simplicity** | Postgres-only (no ClickHouse/Redis) | Multi-store complexity |

### Technology Stack

| Component | Technology | Rationale |
|-----------|------------|-----------|
| **Server** | Go 1.22+ | Performance, single binary |
| **Database** | PostgreSQL 15+ | JSONB flexibility, reliability, simplicity |
| **Queue** | River (Go) | Postgres-native, no Redis dependency |
| **UI** | React + Vite + Tailwind | Modern, embeds in Go binary |
| **SDK** | Python (primary), TypeScript | Agent ecosystem alignment |

---

## 2. Core Architecture Decisions

### 2.1 Single-Store Simplicity

**Decision:** Postgres-only for v1 (no ClickHouse, Redis, or multi-store complexity)

**Rationale:**
- JSONB provides sufficient flexibility for schema evolution
- Materialized views handle analytics workloads
- Partitioning enables scale when needed
- Operational simplicity reduces failure modes
- Can migrate to ClickHouse later if scale demands

**Trade-offs:**
- Higher write latency than dedicated OLAP stores
- Aggregations slower than columnar storage
- Acceptable for MVP scale (millions of spans/month)

### 2.2 Append-Only Event Foundation

**Decision:** `span_events` table is first-class, append-only with idempotency keys

**Rationale:**
- Canonical source of truth for replay
- Enables deterministic re-execution
- Supports streaming (thinking chunks, tool progress)
- Future durable execution foundation

**Implementation:**
- Events never mutated, only appended
- Sequence numbers for ordering within span
- Server-trust timestamps for clock-skew protection

### 2.3 Never-Fail Payload Handling

**Decision:** Wrapper approach for invalid JSON (always store valid JSONB)

**Rationale:**
- Debugger must show what was actually sent
- Rejecting invalid JSON loses critical debugging info
- Simpler queries than dual-column approach

**Implementation:**
```json
// Valid JSON stored as-is
{"messages": [{"role": "user", "content": "Hello"}]}

// Invalid JSON wrapped
{
  "__continua_raw": "malformed{json here",
  "__parse_error": "unexpected EOF at position 15",
  "__content_type": "text/plain"
}
```

### 2.4 Soft Span Integrity

**Decision:** No FK from `span_events` to `spans` (soft integrity via indexes)

**Rationale:**
- Out-of-order ingestion is common (events before spans)
- Orphan events are useful debugging signal
- Avoids skeleton-span garbage data
- Simpler ingestion logic

**Implementation:**
```sql
-- No FK, but efficient joins
CREATE INDEX idx_span_events_span 
    ON span_events(project_id, trace_id, span_id);

-- Orphan query pattern
SELECT e.* FROM span_events e
LEFT JOIN spans s ON s.trace_id = e.trace_id AND s.span_id = e.span_id
WHERE e.trace_id = $1 AND s.id IS NULL;
```

### 2.5 Server-Trust Timestamps

**Decision:** Single `server_received_at` / `server_ingested_at` alongside client times

**Rationale:**
- Client clocks are unreliable
- Server timestamp provides reliable ordering fallback
- Don't duplicate all client timestamps (no `server_start_time`)

**Implementation:**
- `traces.server_received_at` - when trace first ingested
- `spans.server_received_at` - when span first ingested  
- `span_events.server_ingested_at` - when event ingested
- Client `start_time`, `end_time`, `event_ts` preserved for duration calculations

### 2.6 Thinking Capture Strategy

**Decision:** Primary via span_events + denormalized snapshot column

**Rationale:**
- Events support streaming (thinking chunks)
- Snapshot enables fast UI queries
- Avoids schema sprawl (only thinking gets special treatment)

**Implementation:**
- Events: `event_type = 'llm.thinking'` or `'llm.thinking_chunk'`
- Snapshot: `spans.thinking TEXT` updated by worker
- Token counts in `usage_details` / dedicated columns

---

## 3. Data Model

### 3.1 Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CONTINUA DATA MODEL                             │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────┐
│   projects   │
│──────────────│
│ id (PK)      │
│ name         │
│ settings     │
└──────┬───────┘
       │
       │ 1:N
       ▼
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│     api_keys     │     │ redaction_rules  │     │  payload_blobs   │
│──────────────────│     │──────────────────│     │──────────────────│
│ id (PK)          │     │ id (PK)          │     │ id (PK)          │
│ project_id (FK)  │     │ project_id (FK)  │     │ project_id (FK)  │
│ public_key       │     │ name             │     │ sha256 (unique)  │
│ hashed_secret    │     │ pattern          │     │ size_bytes       │
│ scopes[]         │     │ rule_type        │     │ storage_type     │
└──────────────────┘     └──────────────────┘     └──────────────────┘

       │
       │ 1:N
       ▼
┌──────────────────┐
│     sessions     │
│──────────────────│
│ id (PK)          │
│ project_id (FK)  │
│ session_id       │◄─────────── External identifier
│ user_id          │
│ metadata         │
│ [rollups]        │◄─────────── trace_count, total_cost, total_tokens
└──────┬───────────┘
       │
       │ 1:N
       ▼
┌──────────────────────────────────────────────────────────────────┐
│                              traces                               │
│──────────────────────────────────────────────────────────────────│
│ id (PK)                                                          │
│ project_id (FK)                                                  │
│ session_id (FK, nullable)                                        │
│ trace_id                      ◄───────── External identifier     │
│ name, tags[], environment                                        │
│ input, output (JSONB)         ◄───────── Wrapper if invalid      │
│ start_time, end_time          ◄───────── Client-provided         │
│ server_received_at            ◄───────── Server-trust            │
│ status                        ◄───────── running/ok/error        │
│ [rollups]                     ◄───────── total_spans, cost, etc  │
└──────────────────────────────────┬───────────────────────────────┘
                                   │
                                   │ 1:N
                                   ▼
┌──────────────────────────────────────────────────────────────────┐
│                              spans                                │
│──────────────────────────────────────────────────────────────────│
│ id (PK)                                                          │
│ project_id (FK), trace_id (FK)                                   │
│ span_id                       ◄───────── External identifier     │
│ parent_span_id (TEXT)         ◄───────── No FK (out-of-order)    │
│ name, type                    ◄───────── llm/tool/agent/custom   │
│ status, status_message                                           │
│──────────────────────────────────────────────────────────────────│
│ PAYLOADS (with truncation metadata):                             │
│ input, input_truncated, input_original_size_bytes, input_blob_id │
│ output, output_truncated, output_original_size_bytes, ...        │
│ thinking (TEXT snapshot), thinking_truncated, ...                │
│──────────────────────────────────────────────────────────────────│
│ LLM FIELDS: model, provider, usage_details, tokens, cost         │
│ TOOL FIELDS: tool_name, tool_arguments, tool_result              │
│──────────────────────────────────────────────────────────────────│
│ start_time, end_time, server_received_at, duration_ms            │
└──────────────────────────────────┬───────────────────────────────┘
                                   │
                                   │ 1:N (soft integrity - no FK)
                                   ▼
┌──────────────────────────────────────────────────────────────────┐
│                           span_events                             │
│──────────────────────────────────────────────────────────────────│
│ id (PK)                                                          │
│ project_id (FK), trace_id (FK)                                   │
│ span_id (TEXT)                ◄───────── No FK (out-of-order)    │
│ event_type                    ◄───────── Validated at app layer  │
│ level                         ◄───────── debug/info/warn/error   │
│ event_ts, server_ingested_at  ◄───────── Client + server times   │
│ sequence                      ◄───────── Order within span       │
│ message, payload (JSONB)      ◄───────── Wrapper if invalid      │
│ truncated, original_size_bytes, truncation_reason                │
│ idempotency_key               ◄───────── Append-only guarantee   │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────┐     ┌──────────────────────┐
│      scores      │     │  external_trace_ids  │
│──────────────────│     │──────────────────────│
│ id (PK)          │     │ id (PK)              │
│ project_id (FK)  │     │ project_id (FK)      │
│ trace_id (FK)    │     │ system               │
│ span_id (TEXT)   │     │ external_trace_id    │
│ name, data_type  │     │ external_span_id     │
│ value_*          │     │ internal_trace_id    │
│ source, comment  │     │ internal_span_id     │
└──────────────────┘     └──────────────────────┘
```

### 3.2 Span Types

| Type | Description | Special Fields |
|------|-------------|----------------|
| `llm` | LLM API calls | model, provider, tokens, cost, thinking |
| `tool` | Tool/function calls | tool_name, tool_arguments, tool_result |
| `retrieval` | RAG retrieval operations | Retrieved documents in output |
| `agent` | Agent orchestration | Sub-spans for agent steps |
| `custom` | User-defined operations | Flexible metadata |

### 3.3 Span Event Types

Events are validated at application layer, not database. Core types:

**Span Lifecycle:**
- `span.start` - Span execution started
- `span.end` - Span execution completed
- `span.error` - Error occurred
- `span.retry` - Retry attempt

**LLM Events:**
- `llm.request` - Request sent to LLM
- `llm.response` - Response received
- `llm.stream_chunk` - Streaming chunk received
- `llm.thinking` - Thinking/reasoning content
- `llm.thinking_chunk` - Streaming thinking chunk

**Tool Events:**
- `tool.call` - Tool invoked
- `tool.result` - Tool returned result
- `tool.error` - Tool failed

**State Events:**
- `state.change` - State mutation
- `state.checkpoint` - Resumable checkpoint
- `state.snapshot` - Full state snapshot

### 3.4 Event Levels

| Level | Use Case | UI Treatment |
|-------|----------|--------------|
| `debug` | Verbose debugging info | Gray, hidden by default |
| `info` | Normal operation events | Default color |
| `warn` | Warning conditions | Yellow/orange highlight |
| `error` | Error conditions | Red highlight |

---

## 4. System Components & Flows

### 4.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CONTINUA ARCHITECTURE                              │
└─────────────────────────────────────────────────────────────────────────────┘

    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │  Python App │     │  LangChain  │     │   OpenAI    │
    │  + SDK      │     │  + Callback │     │  + Wrapper  │
    └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
           │                   │                   │
           └───────────────────┼───────────────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │   SDK Batch Queue   │
                    │  ┌───────────────┐  │
                    │  │ Start/End     │  │
                    │  │ Merge Logic   │  │
                    │  └───────────────┘  │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │  POST /v1/ingest    │
                    │  (Batch Payload)    │
                    └──────────┬──────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────────────┐
│                         CONTINUA SERVER (Go)                                 │
│                                                                              │
│  ┌────────────────┐    ┌────────────────┐    ┌────────────────┐            │
│  │  Auth          │───▶│  Redaction     │───▶│  Validation    │            │
│  │  Middleware    │    │  + Scrub       │    │  + Truncation  │            │
│  └────────────────┘    └────────────────┘    └───────┬────────┘            │
│                                                       │                      │
│         ┌────────────────────────────────────────────┤                      │
│         │                                            │                      │
│         ▼ (async default)                            ▼ (sync fallback)      │
│  ┌─────────────────┐                         ┌─────────────────┐           │
│  │  River Queue    │                         │  Direct Write   │           │
│  │  (ingest jobs)  │                         │  (debugging)    │           │
│  └───────┬─────────┘                         └───────┬─────────┘           │
│          │                                           │                      │
│          ▼                                           │                      │
│  ┌─────────────────┐                                 │                      │
│  │  Ingest Worker  │◄────────────────────────────────┘                      │
│  │  ┌───────────┐  │                                                        │
│  │  │ Normalize │  │                                                        │
│  │  │ Wrap JSON │  │                                                        │
│  │  │ Blob Check│  │                                                        │
│  │  └───────────┘  │                                                        │
│  └───────┬─────────┘                                                        │
│          │                                                                   │
│          ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────┐       │
│  │                      PostgreSQL                                  │       │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────────┐          │       │
│  │  │ traces  │ │  spans  │ │ events  │ │ payload_blobs│          │       │
│  │  └─────────┘ └─────────┘ └─────────┘ └──────────────┘          │       │
│  └─────────────────────────────────────────────────────────────────┘       │
│                                                                              │
│  ┌─────────────────┐    ┌─────────────────┐                                │
│  │  Query API      │    │  Rollup Worker  │                                │
│  │  GET /v1/...    │    │  (aggregates)   │                                │
│  └───────┬─────────┘    └─────────────────┘                                │
│          │                                                                   │
└──────────┼───────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          WEB UI (React)                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Trace List  │  │ Trace Tree  │  │ Span Detail │  │ Event       │        │
│  │ + Filters   │  │ + Timeline  │  │ + I/O View  │  │ Timeline    │        │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Ingestion Pipeline Detail

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          INGESTION PIPELINE                                  │
└─────────────────────────────────────────────────────────────────────────────┘

SDK Payload
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 1. AUTH                                                        │
│    • Extract API key from Authorization header                 │
│    • Validate key format, lookup in api_keys table            │
│    • Check scopes include 'ingest'                            │
│    • Reject if expired or revoked                              │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 2. SIZE CHECK                                                  │
│    • Reject if payload > 5MB (return 413)                     │
│    • Record server_received_at timestamp                       │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 3. REDACTION                                                   │
│    • Load active redaction rules for project                   │
│    • Apply regex/jsonpath patterns to all payload fields      │
│    • Replace matches with [REDACTED]                           │
│    • Track redaction_rule_version on spans                     │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 4. VALIDATION + WRAPPING                                       │
│    • Validate required fields (trace_id, span_id, name, etc)  │
│    • For each JSONB payload:                                   │
│      ├─ Valid JSON? → Store as-is                             │
│      └─ Invalid? → Wrap: {"__continua_raw": "...",            │
│                           "__parse_error": "..."}             │
│    • Enforce size limits, truncate if needed                   │
│    • Set truncation metadata (original_size, reason)           │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 5. QUEUE OR DIRECT                                             │
│    • Default: Enqueue to River (async, returns 202)           │
│    • ?sync=true: Process inline (debugging, returns 200)      │
└───────────────────────────────────────────────────────────────┘
    │
    ▼ (worker picks up from queue)
┌───────────────────────────────────────────────────────────────┐
│ 6. WRITE TO POSTGRES (Corrected Transaction Order)            │
│                                                                │
│    BEGIN TRANSACTION                                           │
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 1: CLAIM BATCH IDEMPOTENCY (first!)                ││
│    │   INSERT INTO ingest_batches(project_id, batch_key,     ││
│    │          server_received_at, status)                     ││
│    │   VALUES ($1, $2, $3, 'processing')                      ││
│    │   ON CONFLICT(project_id, batch_key) DO NOTHING         ││
│    │   RETURNING id;                                          ││
│    │                                                          ││
│    │   IF no rows returned → ROLLBACK, return {status:       ││
│    │      "duplicate"} (batch already processed)              ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 2: UPSERT TRACES + BUILD ID MAP                    ││
│    │   For each trace in batch:                               ││
│    │     INSERT INTO traces (project_id, trace_id, ...)      ││
│    │     ON CONFLICT(project_id, trace_id) DO UPDATE         ││
│    │     RETURNING id;                                        ││
│    │                                                          ││
│    │   Build map: trace_id_text → trace_uuid                 ││
│    │   (Required because spans.trace_id FK → traces.id)      ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 3: UPSERT SPANS (using trace UUID map)             ││
│    │   For each span in batch:                                ││
│    │     trace_uuid = trace_map[span.trace_id]               ││
│    │     INSERT INTO spans (trace_id=trace_uuid, ...)        ││
│    │     ON CONFLICT(trace_id, span_id) DO UPDATE            ││
│    │     Use COALESCE for patch semantics                     ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 4: INSERT EVENTS (append-only, using trace UUID)   ││
│    │   For each event in batch:                               ││
│    │     trace_uuid = trace_map[event.trace_id]              ││
│    │     INSERT INTO span_events (trace_id=trace_uuid, ...)  ││
│    │     ON CONFLICT(project_id, idempotency_key) DO NOTHING ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 5: UPDATE THINKING SNAPSHOTS                       ││
│    │   For spans with thinking events:                        ││
│    │     UPDATE spans SET thinking = (latest thinking text)  ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    ┌─────────────────────────────────────────────────────────┐│
│    │ STEP 6: UPDATE BATCH STATUS                             ││
│    │   UPDATE ingest_batches SET                              ││
│    │     status = 'accepted',                                 ││
│    │     trace_count = X, span_count = Y, event_count = Z    ││
│    │   WHERE id = $batch_id;                                  ││
│    └─────────────────────────────────────────────────────────┘│
│                                                                │
│    COMMIT                                                      │
│                                                                │
└───────────────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────────────┐
│ 7. ROLLUP WORKER (async, separate job)                        │
│    • Update trace aggregates (total_spans, cost, tokens)      │
│    • Update session aggregates                                 │
│    • Compute duration from span times                          │
└───────────────────────────────────────────────────────────────┘
```

**Critical Implementation Notes:**

1. **Batch claim must be first** - If batch_key already exists, exit immediately (duplicate request). This prevents double-processing.

2. **Trace ID mapping is required** because:
   - SDK sends `trace_id` as TEXT (external identifier)
   - DB stores `traces.id` as UUID (internal PK)
   - `spans.trace_id` and `span_events.trace_id` are FK to `traces.id` (UUID)
   - Worker must map external → internal before inserting spans/events

3. **Timestamp semantics:**
   - `server_received_at` = time of database write (includes queue delay)
   - For v1, this is acceptable; primary purpose is clock-skew protection
   - Client `start_time`/`end_time` used for duration calculations

### 4.3 Query Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                             QUERY PATTERNS                                   │
└─────────────────────────────────────────────────────────────────────────────┘

TRACE LIST (Two-Query Pattern)
══════════════════════════════

Request: GET /v1/traces?environment=prod&status=error&limit=50

Query 1: Core List (fast)
┌────────────────────────────────────────────────────────────────┐
│ SELECT id, trace_id, name, status, start_time, duration_ms,   │
│        total_spans, total_cost, total_tokens, error_count     │
│ FROM traces                                                    │
│ WHERE project_id = $1 AND environment = $2 AND status = $3    │
│ ORDER BY server_received_at DESC                               │
│ LIMIT 50                                                       │
└────────────────────────────────────────────────────────────────┘

Query 2: Metrics (separate, can be cached)
┌────────────────────────────────────────────────────────────────┐
│ SELECT COUNT(*), SUM(total_cost), AVG(duration_ms)            │
│ FROM traces                                                    │
│ WHERE project_id = $1 AND environment = $2 AND status = $3    │
└────────────────────────────────────────────────────────────────┘


TRACE DETAIL (Lazy Loading)
═══════════════════════════

Step 1: Load trace + spans (no I/O payloads)
┌────────────────────────────────────────────────────────────────┐
│ SELECT t.*, s.id, s.span_id, s.parent_span_id, s.name,        │
│        s.type, s.status, s.start_time, s.end_time,            │
│        s.model, s.total_cost, s.total_tokens                   │
│ FROM traces t                                                  │
│ LEFT JOIN spans s ON s.trace_id = t.id                        │
│ WHERE t.id = $1                                                │
│ ORDER BY s.start_time                                          │
└────────────────────────────────────────────────────────────────┘

Step 2: Load full span on selection (includes I/O)
┌────────────────────────────────────────────────────────────────┐
│ SELECT * FROM spans WHERE id = $1                              │
└────────────────────────────────────────────────────────────────┘

Step 3: Load events for selected span
┌────────────────────────────────────────────────────────────────┐
│ SELECT * FROM span_events                                      │
│ WHERE trace_id = $1 AND span_id = $2                          │
│ ORDER BY COALESCE(event_ts, server_ingested_at), sequence     │
└────────────────────────────────────────────────────────────────┘


ORPHAN EVENTS (Debugging)
═════════════════════════

Find events without matching spans:
┌────────────────────────────────────────────────────────────────┐
│ SELECT e.* FROM span_events e                                  │
│ LEFT JOIN spans s                                              │
│   ON s.trace_id = e.trace_id AND s.span_id = e.span_id        │
│ WHERE e.trace_id = $1 AND s.id IS NULL                        │
└────────────────────────────────────────────────────────────────┘


TREE BUILDING (Client-Side)
═══════════════════════════

UI receives flat span list, builds tree:

function buildTree(spans) {
  const byId = new Map(spans.map(s => [s.span_id, {...s, children: []}]));
  const roots = [];
  
  for (const span of byId.values()) {
    if (!span.parent_span_id || !byId.has(span.parent_span_id)) {
      // Root or orphan (parent missing) - treat as root
      roots.push(span);
    } else {
      byId.get(span.parent_span_id).children.push(span);
    }
  }
  
  return roots;
}
```

---

## 5. API Design

### 5.1 Endpoints Overview

**Important:** Since `span_id` uniqueness is scoped to `(trace_id, span_id)`, span endpoints must include trace context. We use nested routes for clarity.

| Method | Endpoint | Description |
|--------|----------|-------------|
| **Ingestion** |||
| POST | `/v1/ingest` | Batch ingest traces, spans, events, scores |
| POST | `/v1/otel/traces` | OTLP ingestion (protobuf/JSON) |
| **Traces** |||
| GET | `/v1/traces` | List traces with filters |
| GET | `/v1/traces/{trace_id}` | Get trace detail with span summaries |
| **Spans** (nested under trace for uniqueness) |||
| GET | `/v1/traces/{trace_id}/spans` | List spans for trace |
| GET | `/v1/traces/{trace_id}/spans/{span_id}` | Get span with full payload |
| GET | `/v1/spans/{span_uuid}` | Get span by internal UUID (alternative) |
| **Events** |||
| GET | `/v1/traces/{trace_id}/spans/{span_id}/events` | List events for span |
| GET | `/v1/traces/{trace_id}/events` | List all events for trace |
| GET | `/v1/traces/{trace_id}/orphan-events` | List orphan events (no matching span) |
| POST | `/v1/span-events` | Append events (batch) |
| **Sessions** |||
| GET | `/v1/sessions` | List sessions |
| GET | `/v1/sessions/{session_id}` | Get session with traces |
| POST | `/v1/sessions` | Create/update session |
| **Scores** |||
| GET | `/v1/scores` | List scores |
| POST | `/v1/scores` | Create score |
| **Export** |||
| GET | `/v1/export/traces/{trace_id}` | Export trace for replay |
| **Admin** |||
| GET | `/v1/projects` | List projects |
| POST | `/v1/projects` | Create project |
| GET/POST/DELETE | `/v1/api-keys` | Manage API keys |
| **Operations** |||
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check |
| GET | `/metrics` | Prometheus metrics |

**Note on Span Identification:**
- External span IDs (`span_id` TEXT) are only unique within a trace
- Internal UUIDs (`spans.id`) are globally unique
- Trace detail response includes both: `{span_id: "sp_001", span_uuid: "uuid-here", ...}`
- UI can use `span_uuid` for direct lookups after initial load

### 5.2 Batch Ingest Request

```json
POST /v1/ingest
Content-Type: application/json
Authorization: Bearer pk_xxx:sk_xxx

{
  "batch_key": "batch_abc123",  // Idempotency key for entire batch
  "sdk_info": {
    "name": "continua-python",
    "version": "0.1.0",
    "language": "python"
  },
  "traces": [
    {
      "trace_id": "tr_xyz",
      "name": "chat_completion",
      "start_time": "2025-01-08T10:00:00Z",
      "tags": ["production", "gpt-4"],
      "metadata": {"user_id": "u_123"}
    }
  ],
  "spans": [
    {
      "trace_id": "tr_xyz",
      "span_id": "sp_001",
      "parent_span_id": null,
      "name": "openai.chat",
      "type": "llm",
      "start_time": "2025-01-08T10:00:00Z",
      "end_time": "2025-01-08T10:00:02Z",
      "model": "gpt-4",
      "input": {"messages": [{"role": "user", "content": "Hello"}]},
      "output": {"content": "Hi there!"},
      "usage": {"prompt_tokens": 10, "completion_tokens": 5}
    }
  ],
  "events": [
    {
      "trace_id": "tr_xyz",
      "span_id": "sp_001",
      "event_type": "llm.response",
      "level": "info",
      "event_ts": "2025-01-08T10:00:02Z",
      "message": "Response received",
      "payload": {"latency_ms": 1523},
      "idempotency_key": "evt_abc"
    }
  ],
  "scores": [
    {
      "trace_id": "tr_xyz",
      "name": "quality",
      "value_numeric": 0.95,
      "source": "evaluator"
    }
  ]
}
```

### 5.3 Ingest Response

**HTTP Status Codes:**

| Scenario | Status | Response |
|----------|--------|----------|
| Async, new batch | 202 | `{status: "accepted", batch_key: "..."}` |
| Async, duplicate batch | 202 | `{status: "duplicate", batch_key: "..."}` |
| Sync, success | 200 | `{status: "ok", items: [...]}` |
| Sync, duplicate batch | 200 | `{status: "duplicate"}` |
| Validation error | 400 | `{error: "...", details: [...]}` |
| Auth failure | 401/403 | `{error: "..."}` |
| Batch too large | 413 | `{error: "batch exceeds 5MB limit"}` |

**Note:** Duplicate batches return success (200/202), not 409. A duplicate means "already processed successfully" which is a success from the client's perspective.

**Async Response (202):**
```json
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "status": "accepted",
  "batch_key": "batch_abc123",
  "received_at": "2025-01-08T10:00:03Z"
}
```

**Sync Response (200):**
```json
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "batch_key": "batch_abc123",
  "received_at": "2025-01-08T10:00:03Z",
  "counts": {
    "traces": 1,
    "spans": 1,
    "events": 1,
    "scores": 1
  },
  "items": [
    {"type": "trace", "id": "tr_xyz", "status": "created"},
    {"type": "span", "id": "sp_001", "span_uuid": "uuid-here", "status": "created"},
    {"type": "event", "id": "evt_abc", "status": "created"},
    {"type": "score", "id": "...", "status": "created"}
  ]
}
```

**Batch Size Limit:** Maximum batch payload is 5MB. Larger batches receive 413 error.

### 5.4 Idempotency Strategy

| Entity | Key | Behavior |
|--------|-----|----------|
| Batch | `(project_id, batch_key)` | Reject duplicate batch |
| Trace | `(project_id, trace_id)` | Upsert with merge |
| Span | `(trace_id, span_id)` | Upsert with patch |
| Event | `(project_id, idempotency_key)` | Insert or ignore |
| Score | `(project_id, idempotency_key)` | Upsert |

### 5.5 Upsert Patch Semantics

For spans arriving out-of-order (start event, then end event):

```sql
INSERT INTO spans (trace_id, span_id, name, start_time, status, ...)
VALUES ($1, $2, $3, $4, 'running', ...)
ON CONFLICT (trace_id, span_id) DO UPDATE SET
  -- Don't overwrite with NULL
  name = COALESCE(EXCLUDED.name, spans.name),
  -- Latest timestamp wins
  end_time = GREATEST(spans.end_time, EXCLUDED.end_time),
  -- Error status wins
  status = CASE 
    WHEN EXCLUDED.status = 'error' THEN 'error'
    WHEN spans.status = 'error' THEN 'error'
    ELSE COALESCE(EXCLUDED.status, spans.status)
  END,
  -- Accumulate tokens
  total_tokens = COALESCE(EXCLUDED.total_tokens, 0) + COALESCE(spans.total_tokens, 0),
  version = spans.version + 1
WHERE spans.version < EXCLUDED.version  -- Optimistic concurrency
```

---

## 6. SDK Design

### 6.1 Python SDK Architecture

```
continua-python/
├── continua/
│   ├── __init__.py           # Public API exports
│   ├── client.py             # ContinuaClient main class
│   ├── config.py             # Configuration
│   │
│   ├── core/
│   │   ├── context.py        # Trace/span context management
│   │   ├── tracer.py         # OpenTelemetry-style API
│   │   ├── decorators.py     # @trace, @span decorators
│   │   └── models.py         # Pydantic models
│   │
│   ├── batch/
│   │   ├── queue.py          # In-memory batch queue
│   │   ├── batcher.py        # Message batching + merge
│   │   └── worker.py         # Background flush worker
│   │
│   ├── transport/
│   │   ├── http.py           # HTTP transport
│   │   └── retry.py          # Exponential backoff
│   │
│   └── integrations/
│       ├── langchain.py      # LangChain callback handler
│       ├── openai.py         # OpenAI wrapper
│       ├── anthropic.py      # Anthropic wrapper
│       └── llamaindex.py     # LlamaIndex callbacks
│
├── tests/
├── pyproject.toml
└── README.md
```

### 6.2 SDK Usage Examples

**Basic Usage:**
```python
from continua import ContinuaClient

client = ContinuaClient(
    api_key="pk_xxx:sk_xxx",
    environment="production"
)

# Context manager
with client.trace(name="user_query") as trace:
    with trace.span(name="retrieve_docs", type="retrieval") as span:
        docs = retriever.search(query)
        span.set_output(docs)
    
    with trace.span(name="generate_response", type="llm") as span:
        span.set_input({"messages": messages})
        response = llm.generate(messages)
        span.set_output(response)
        span.set_usage(prompt_tokens=100, completion_tokens=50)

# Decorators
@client.trace()
def process_query(query: str):
    
    @client.span(type="llm")
    def call_llm(messages):
        return openai.chat.completions.create(...)
    
    return call_llm([{"role": "user", "content": query}])
```

**LLM Helper:**
```python
with client.trace(name="chat") as trace:
    with trace.llm_span(
        name="gpt4_call",
        model="gpt-4",
        input={"messages": messages}
    ) as span:
        response = openai.chat.completions.create(
            model="gpt-4",
            messages=messages
        )
        span.complete(
            output=response.choices[0].message,
            usage=response.usage
        )
```

**Events:**
```python
with trace.span(name="agent_loop", type="agent") as span:
    for step in range(max_steps):
        span.add_event(
            type="state.checkpoint",
            level="info",
            message=f"Step {step}",
            payload={"step": step, "state": current_state}
        )
        
        if has_error:
            span.add_event(
                type="span.error",
                level="error",
                message="Tool failed",
                payload={"error": str(e)}
            )
```

### 6.3 SDK Batching Strategy

```python
class BatchQueue:
    """
    Batching with start/end merge (Opik pattern)
    
    IMPORTANT: Never create "end-only" spans. If end arrives without 
    a corresponding start, log warning and drop the data. The DB
    requires start_time and name which only come from span start.
    """
    
    def __init__(
        self,
        max_batch_size: int = 100,
        max_batch_bytes: int = 1_000_000,  # 1MB
        flush_interval_seconds: float = 1.0
    ):
        self._traces: dict[str, TraceData] = {}
        self._spans: dict[str, SpanData] = {}
        self._events: list[EventData] = []
        self._lock = threading.Lock()
        
    def enqueue_span_start(self, span: SpanData):
        """Record span start - this creates the span entry."""
        with self._lock:
            key = f"{span.trace_id}:{span.span_id}"
            self._spans[key] = span
            
    def enqueue_span_end(self, trace_id: str, span_id: str, **updates):
        """
        Record span end - merges into existing span.
        
        If no start exists, logs warning and drops data.
        This prevents creating invalid spans without required fields.
        """
        with self._lock:
            key = f"{trace_id}:{span_id}"
            if key in self._spans:
                # Merge end data into existing start
                self._spans[key].merge_end(**updates)
            else:
                # No start received - cannot create valid span
                # Log warning but don't crash
                logger.warning(
                    f"Span end received without start: {span_id}. "
                    f"Data dropped. Ensure span() context manager "
                    f"is used correctly."
                )
                # DO NOT create a partial span here
    
    def flush(self) -> BatchPayload:
        with self._lock:
            payload = BatchPayload(
                traces=list(self._traces.values()),
                spans=list(self._spans.values()),
                events=self._events.copy()
            )
            self._traces.clear()
            self._spans.clear()
            self._events.clear()
            return payload
```

**Why no end-only spans?**

The database schema requires:
- `spans.name TEXT NOT NULL`
- `spans.start_time TIMESTAMPTZ NOT NULL`

These fields only come from span start. Creating a span from end-only data would require:
- Placeholder name (garbage data)
- Guessed start_time (incorrect)

Both violate the "debugger shows real data" principle. Better to drop orphan ends and log a warning so developers can fix their instrumentation.

### 6.4 SDK Retry Strategy

```python
class RetryTransport:
    """
    Exponential backoff with jitter, respects 429 Retry-After
    """
    
    def __init__(
        self,
        max_retries: int = 3,
        base_delay: float = 0.5,
        max_delay: float = 30.0
    ):
        self.max_retries = max_retries
        self.base_delay = base_delay
        self.max_delay = max_delay
        
    def send(self, payload: bytes) -> Response:
        for attempt in range(self.max_retries + 1):
            try:
                response = self._http.post("/v1/ingest", payload)
                
                if response.status_code == 429:
                    retry_after = response.headers.get("Retry-After", 
                                                        self._backoff(attempt))
                    time.sleep(float(retry_after))
                    continue
                    
                if response.status_code >= 500:
                    time.sleep(self._backoff(attempt))
                    continue
                    
                return response
                
            except (ConnectionError, Timeout):
                if attempt == self.max_retries:
                    raise
                time.sleep(self._backoff(attempt))
                
    def _backoff(self, attempt: int) -> float:
        delay = self.base_delay * (2 ** attempt)
        jitter = random.uniform(0, delay * 0.1)
        return min(delay + jitter, self.max_delay)
```

---

## 7. UI Architecture

### 7.1 Component Structure

```
ui/src/
├── pages/
│   ├── traces/
│   │   ├── TracesListPage.tsx      # Main trace list with filters
│   │   └── TraceDetailPage.tsx     # Trace detail with tree/timeline
│   ├── sessions/
│   │   ├── SessionsListPage.tsx
│   │   └── SessionDetailPage.tsx
│   └── settings/
│       ├── ProjectSettingsPage.tsx
│       └── ApiKeysPage.tsx
│
├── components/
│   ├── trace/
│   │   ├── TraceTree.tsx           # Virtualized span tree
│   │   ├── TraceTimeline.tsx       # Timeline bar visualization
│   │   ├── SpanInspector.tsx       # Selected span detail panel
│   │   ├── SpanEvents.tsx          # Events timeline for span
│   │   ├── LLMMessagesView.tsx     # Chat bubble rendering
│   │   ├── ToolCallView.tsx        # Tool call visualization
│   │   ├── ThinkingView.tsx        # Thinking/reasoning display
│   │   └── OrphanEventsPanel.tsx   # Orphan events display
│   │
│   ├── table/
│   │   ├── DataTable.tsx           # Sortable, filterable table
│   │   ├── FilterBar.tsx           # Filter controls
│   │   ├── Pagination.tsx
│   │   └── TimeRangePicker.tsx
│   │
│   ├── json/
│   │   ├── JsonViewer.tsx          # Syntax highlighted JSON
│   │   ├── JsonDiff.tsx            # Input/output diff
│   │   └── TruncatedPayload.tsx    # Truncation indicator
│   │
│   └── common/
│       ├── CopyButton.tsx
│       ├── StatusBadge.tsx
│       ├── EventLevelBadge.tsx
│       └── TokenCostDisplay.tsx
│
├── hooks/
│   ├── useTraces.ts               # Trace list with SWR
│   ├── useTraceDetail.ts          # Single trace + spans
│   ├── useSpanDetail.ts           # Full span with payload
│   ├── useSpanEvents.ts           # Events for span
│   └── useOrphanEvents.ts         # Orphan events for trace
│
├── utils/
│   ├── tree.ts                    # Build tree from flat spans
│   ├── timeline.ts                # Timeline calculations
│   ├── format.ts                  # Token/cost formatting
│   └── json-worker.ts             # Web worker for large JSON
│
└── styles/
    └── globals.css
```

### 7.2 Key UI Patterns

**Virtualized Tree (for large traces):**
```tsx
import { VariableSizeTree } from 'react-vtree';

function TraceTree({ spans }: { spans: Span[] }) {
  const tree = useMemo(() => buildTree(spans), [spans]);
  
  return (
    <VariableSizeTree
      treeWalker={treeWalker(tree)}
      itemSize={getNodeHeight}
      height={600}
      width="100%"
    >
      {SpanNode}
    </VariableSizeTree>
  );
}
```

**Lazy Payload Loading:**
```tsx
function SpanInspector({ span }: { span: SpanSummary }) {
  // Only load full payload when span selected
  const { data: fullSpan } = useSWR(
    span.id ? `/api/spans/${span.id}` : null
  );
  
  if (!fullSpan) return <Skeleton />;
  
  return (
    <div>
      <JsonViewer 
        data={fullSpan.input} 
        truncated={fullSpan.input_truncated}
        originalSize={fullSpan.input_original_size_bytes}
      />
    </div>
  );
}
```

**Web Worker for Large JSON:**
```ts
// json-worker.ts
self.onmessage = (e) => {
  const { json } = e.data;
  try {
    const parsed = JSON.parse(json);
    const formatted = formatJson(parsed);
    self.postMessage({ success: true, result: formatted });
  } catch (error) {
    self.postMessage({ success: false, error: error.message });
  }
};
```

**Orphan Events Panel:**
```tsx
function OrphanEventsPanel({ traceId }: { traceId: string }) {
  const { data: orphans } = useOrphanEvents(traceId);
  
  if (!orphans?.length) return null;
  
  return (
    <Collapsible className="border-yellow-200 bg-yellow-50">
      <CollapsibleTrigger>
        ⚠️ {orphans.length} orphan events (spans not received)
      </CollapsibleTrigger>
      <CollapsibleContent>
        <EventsTimeline events={orphans} />
      </CollapsibleContent>
    </Collapsible>
  );
}
```

### 7.3 Timeline Visualization

```tsx
function TraceTimeline({ spans, traceDuration }: Props) {
  return (
    <div className="relative">
      {spans.map(span => {
        const left = (span.start_time - traceStart) / traceDuration * 100;
        const width = span.duration_ms / traceDuration * 100;
        
        return (
          <div
            key={span.id}
            className="absolute h-6 rounded"
            style={{
              left: `${left}%`,
              width: `${Math.max(width, 0.5)}%`,
              backgroundColor: getSpanColor(span.type, span.status),
              marginTop: span.depth * 28
            }}
          >
            <span className="truncate text-xs">{span.name}</span>
          </div>
        );
      })}
    </div>
  );
}

function getSpanColor(type: string, status: string): string {
  if (status === 'error') return '#ef4444';  // red
  switch (type) {
    case 'llm': return '#3b82f6';      // blue
    case 'tool': return '#10b981';      // green
    case 'retrieval': return '#8b5cf6'; // purple
    case 'agent': return '#f59e0b';     // amber
    default: return '#6b7280';          // gray
  }
}
```

---

## 8. Durability & Replay Foundation

### 8.1 Event Log as Source of Truth

The `span_events` table enables:

1. **Deterministic Replay** - Re-execute traces with exact same inputs
2. **Crash Recovery** - Resume from last checkpoint
3. **Time-Travel Debugging** - Step through execution history
4. **Streaming Capture** - Record thinking chunks, tool progress

### 8.2 Replay Architecture (v1.5+)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          REPLAY ARCHITECTURE                                 │
└─────────────────────────────────────────────────────────────────────────────┘

Original Execution                          Replay Execution
═══════════════════                         ════════════════════

span_events (captured)                      span_events (replayed)
┌───────────────────┐                       ┌───────────────────┐
│ llm.request       │──────────────────────▶│ llm.request       │
│ llm.thinking      │                       │ [mock response]   │
│ llm.response      │                       │ llm.thinking      │
│ tool.call         │──────────────────────▶│ llm.response      │
│ tool.result       │                       │ tool.call         │
│ state.checkpoint  │◀──── Resume point     │ [mock result]     │
│ ...               │                       │ tool.result       │
└───────────────────┘                       └───────────────────┘

Export: GET /v1/export/traces/{id}
┌─────────────────────────────────────────┐
│ {                                        │
│   "trace": {...},                       │
│   "spans": [...],                       │
│   "events": [...],    ◀── Full history  │
│   "mocks": {          ◀── Captured I/O  │
│     "llm": [...],                       │
│     "tools": [...]                      │
│   }                                      │
│ }                                        │
└─────────────────────────────────────────┘
```

### 8.3 Future Tables (Post-v1)

```sql
-- Execution runs for tracking replay attempts
CREATE TABLE execution_runs (
    id UUID PRIMARY KEY,
    original_trace_id UUID REFERENCES traces(id),
    run_number INT,
    status TEXT,  -- running, completed, failed, paused
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    modifications JSONB,  -- What was changed for this run
    UNIQUE(original_trace_id, run_number)
);

-- Checkpoints for resumable execution
CREATE TABLE checkpoints (
    id UUID PRIMARY KEY,
    trace_id UUID REFERENCES traces(id),
    span_id TEXT,
    sequence INT,
    state_snapshot JSONB,
    created_at TIMESTAMPTZ
);
```

---

## 9. Implementation Phases

### Phase 1: Foundation (Weeks 1-3)
**Goal:** Core schema, basic ingestion, minimal query API

- Sprint 1.1: Database foundation (schema, migrations, connection pool)
- Sprint 1.2: API foundation (HTTP server, auth, health endpoints)
- Sprint 1.3: Ingestion & query (POST /v1/ingest, GET /v1/traces)

**Exit Criteria:** Can ingest traces/spans/events, query list/detail

### Phase 2: SDK & Batching (Weeks 4-5)
**Goal:** Python SDK with batching and retry

- Python SDK skeleton (trace/span context, batching)
- River queue integration
- Async ingest worker
- SDK retry with exponential backoff
- Idempotency support

**Exit Criteria:** SDK traces Python code, batching works, retries handle failures

### Phase 3: Web UI (Weeks 6-8)
**Goal:** Functional debugging interface

- UI scaffolding (Vite + React + Tailwind)
- Trace list with filters/pagination
- Trace detail with span tree
- Span inspector with I/O view
- Events timeline
- Embed UI in Go binary

**Exit Criteria:** Can browse traces, inspect spans, view events

### Phase 4: Payload Management (Weeks 9-10)
**Goal:** Handle large payloads gracefully

- Blob storage implementation
- Size threshold detection
- Truncation with metadata
- Lazy loading in UI
- Cleanup worker

**Exit Criteria:** Large payloads stored as blobs, UI loads lazily

### Phase 5: Sessions & Scores (Weeks 11-12)
**Goal:** Grouping and annotations

- Session CRUD endpoints
- Session-trace linking
- Score CRUD endpoints
- Score display in UI

**Exit Criteria:** Traces grouped into sessions, scores annotate traces

### Phase 6: OTel & Integrations (Weeks 13-14)
**Goal:** Ecosystem compatibility

- OTLP HTTP endpoint
- OTel → Continua mapping
- LangChain callback handler
- OpenAI/Anthropic wrappers

**Exit Criteria:** Can ingest OTel traces, framework integrations work

### Phase 7: Production Polish (Weeks 15-16)
**Goal:** Production readiness

- Redaction rules implementation
- Rate limiting
- Prometheus metrics
- Performance optimization
- Documentation
- Deployment guide

**Exit Criteria:** Production-ready, documented, deployable

---

## Appendix: Key Technical Decisions

| Decision | Choice | Rationale | Alternative Considered |
|----------|--------|-----------|------------------------|
| Database | Postgres-only | Operational simplicity, JSONB flexibility | ClickHouse (deferred) |
| Queue | River (Go) | Postgres-native, no Redis | BullMQ, SQS |
| Invalid JSON | Wrapper approach | Never-fail, simple queries | Dual columns, reject |
| span_events FK | No FK (soft integrity) | Out-of-order tolerance | Skeleton upserts |
| Thinking capture | Events + snapshot | Streaming + fast queries | Dedicated columns only |
| Event type constraint | App layer validation | Flexibility, evolution | DB CHECK constraint |
| Server timestamps | Single received_at | Clock-skew protection | Full timestamp duplication |
| Span ID scope | (trace_id, span_id) composite | Enables replay | Global span_id |
| API style | REST | Simplicity | GraphQL |
| SDK batching | Start/end merge | Reduces API calls | Immediate flush |

---

## Document History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-01-08 | Initial v1 architecture with all agreed decisions |

---

*End of Architecture Plan*
