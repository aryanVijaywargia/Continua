> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Continua: AI Agent Observability Platform
## Comprehensive Project Report (Corrected)

**Document Version**: 2.1
**Date**: March 2026
**Author**: Aryan (Founder/Developer)
**Verified By**: Codex audit pass
**Purpose**: Single source of truth for Continua development, verified against actual implementation

---

# Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Project Vision & Value Proposition](#2-project-vision--value-proposition)
3. [Technical Architecture](#3-technical-architecture)
4. [Database Schema (Verified)](#4-database-schema-verified)
5. [API Surface (Verified)](#5-api-surface-verified)
6. [Domain Types & Status Model](#6-domain-types--status-model)
7. [Implementation Status by Phase](#7-implementation-status-by-phase)
8. [Phase 3 Feature Details (Verified)](#8-phase-3-feature-details-verified)
9. [Known Gaps & Phase 4 Planning Inputs](#9-known-gaps--phase-4-planning-inputs)
10. [Development Conventions](#10-development-conventions)
11. [Key Architectural Decisions](#11-key-architectural-decisions)
12. [Reference Implementations Studied](#12-reference-implementations-studied)
13. [Appendix: File Structure](#13-appendix-file-structure)
14. [Phase 3 Final Fixes (Superseding Update)](#14-phase-3-final-fixes-superseding-update)

---

# 1. Executive Summary

## What is Continua?

Continua is an **AI Agent Observability and Debugging Platform** designed to help developers understand "where exactly did my agent go wrong, and what data caused it."

## Core Value Proposition

| Problem | Continua's Solution |
|---------|---------------------|
| AI agents are black boxes | Full trace visualization with span trees |
| "My agent failed but I don't know why" | Capture inputs/outputs at every step |
| Debugging requires code changes | SDK auto-instruments with decorators |
| Cost tracking is manual | Automatic token counting and cost aggregation |
| No session continuity | Session-based trace grouping |

## Strategic Pivot History

**Original Concept**: Full agent runtime (like "Temporal for AI agents")
**Current Focus**: Observability platform that integrates with existing frameworks

This pivot reduced complexity by ~70% while maintaining core value.

## Current State (Post Phase 3 - Verified)

| Component | Status | Notes |
|-----------|--------|-------|
| Server bootstrap | ✅ Done | Fx modules wired |
| API key authentication | ✅ Done | SHA-256 hash lookup |
| Ingest pipeline | ✅ Done | Batch idempotency, validation |
| Async rollups | ✅ Done | River jobs, transactional enqueue |
| Search & filtering | ✅ Done | Full-text search, multi-field filters |
| Sessions API/UI | ✅ Done | List/detail flows (read-only) |
| Python SDK | ✅ Done | Retries, exceptions, session context |
| TypeScript SDK | ⚠️ Scaffold | Not feature parity |
| WebSocket real-time | ⚠️ Contracts only | Backend pipeline incomplete |

## Important Implementation Details

These facts were verified by code audit and differ from some planning assumptions:

1. **Sessions table** uses internal UUID `id` with optional `name`/`user_id`/`metadata`; does NOT store external `session_id` text field
2. **Traces** reference sessions via `session_id` UUID FK to `sessions.id`
3. **Trace rollups** in DB are `total_spans`, `total_tokens`, `total_cost`, `error_count`
4. **API token fields** (`total_tokens_in`/`total_tokens_out`) are derived in mapper for compatibility
5. **API key auth** uses SHA-256 hash lookup (`api_key_hash` column)
6. **Sessions API** supports pagination only (`limit`, `offset`), no `user_id` filter yet
7. **Session creation** is not exposed as dedicated API; happens implicitly or via ingest

### Phase 3 Final Fixes (Superseding Notes)

The following were implemented after the earlier Phase 3 report content above.  
If any statement above conflicts with this block, this block is authoritative.

1. **Sessions now include `external_id`**:
   - `sessions.external_id TEXT NOT NULL`
   - unique index on `(project_id, external_id)`
   - ingest resolves/creates session by external key.
2. **Trace rollups are now split in DB**:
   - `traces.total_tokens_in`
   - `traces.total_tokens_out`
   - old `traces.total_tokens` removed.
3. **API token fields are no longer synthetic compatibility splits**:
   - mapper returns `total_tokens_in` and `total_tokens_out` directly from DB columns.
4. **Ingest trace session ID contract changed**:
   - `IngestTraceInput.session_id` is plain string external key (not UUID-formatted).
5. **Session API contract now includes external ID**:
   - session list/detail responses include `external_id`.
6. **Ingest token policy tightened**:
   - spans with `total_tokens` only (without directional tokens) are rejected.
7. **Python SDK alignment fix completed**:
   - no `total_tokens` emission from span payload builder
   - `set_tokens(total=...)` without prompt/completion now raises a client-side error.
8. **Runtime startup fixes completed**:
   - River worker startup context fixed to prevent notifier reconnect/error loops
   - Vite proxy target aligned with backend default port (`8080`).

---

# 2. Project Vision & Value Proposition

## Target Users

1. **AI Agent Developers**: Building with LangChain, CrewAI, AutoGPT, custom frameworks
2. **ML Engineers**: Monitoring LLM costs and performance
3. **DevOps/Platform Teams**: Operating AI systems in production

## Key Features (Current State)

### Observability
- **Trace Capture**: Every agent execution becomes a trace
- **Span Tree**: Hierarchical view of LLM calls, tool uses, sub-agents
- **Input/Output Recording**: Full request/response payloads (with truncation)
- **Metadata**: Custom tags, user IDs, session references

### Analytics
- **Cost Tracking**: Per-trace cost aggregation
- **Token Usage**: Aggregate token counting
- **Performance**: Duration tracking via timestamps

### Integration
- **SDK-First**: Python SDK (production-ready), TypeScript SDK (scaffold)
- **Framework Agnostic**: Works with any agent framework
- **Zero-Config Start**: Decorator-based instrumentation

## Scope Boundary at End of Phase 3

| In Scope (Implemented) | Out of Scope / Incomplete |
|------------------------|---------------------------|
| Ingestion reliability | Full realtime backend |
| Async rollups via River | TypeScript SDK parity |
| Search and filtering | Enterprise controls (RBAC/SSO) |
| Session read UX | Explicit session create API |
| Python SDK hardening | Benchmark/SLO infrastructure |

---

# 3. Technical Architecture

## Tech Stack

| Layer | Technology | Version |
|-------|------------|---------|
| **Backend** | Go | 1.22+ |
| **DI Framework** | Uber Fx | Latest |
| **HTTP Router** | Chi | v5 |
| **Database** | PostgreSQL | 16 (min 14) |
| **Job Queue** | River | Latest |
| **DB Driver** | pgx | v5 |
| **SQL Generator** | sqlc | 1.30+ |
| **API Contract** | OpenAPI | 3.1 |
| **Code Gen** | oapi-codegen | Latest |
| **Python SDK** | httpx, pydantic | 3.11+ |
| **Web UI** | React, Vite, TypeScript | 18, 5.x, 5.6 |
| **UI State** | TanStack Query | v5 |
| **UI Styling** | Tailwind CSS | v3 |
| **CI/CD** | GitHub Actions | - |
| **Review** | CodeRabbit | - |

## Architecture Principles

### 1. Contract-First Development
```
OpenAPI Spec (source of truth)
    ↓ make generate
Go Server Types + TypeScript Types
    ↓
Implementation
```

### 2. Fx Module Structure
```
cmd/continua/main.go
    ↓ fx.New()
┌────────┬────────┬────────┬────────┬────────┐
│ config │ store  │ jobs   │ ingest │  api   │
└────────┴────────┴────────┴────────┴────────┘
```

### 3. Layered Architecture
```
┌─────────────────────────────────────┐
│           HTTP Layer (Chi)          │
│         internal/api/               │
├─────────────────────────────────────┤
│         Service Layer               │
│       internal/ingest/              │
├─────────────────────────────────────┤
│          Jobs Layer                 │
│        internal/jobs/               │
├─────────────────────────────────────┤
│          Store Layer                │
│        internal/store/              │
├─────────────────────────────────────┤
│         Database (PostgreSQL)       │
│    db/platform/migrations/          │
└─────────────────────────────────────┘
```

### 4. Router Composition Pattern
```go
r := chi.NewRouter()

// Global middleware
r.Use(middleware.RequestID)
r.Use(middleware.Logger)
r.Use(middleware.Recoverer)

// Public routes (NOT in OpenAPI)
r.Get("/api/health", handlers.Health)

// Protected routes (from OpenAPI)
r.Group(func(r chi.Router) {
    r.Use(middleware.APIKeyAuth(store))  // SHA-256 hash lookup
    HandlerWithOptions(server, ChiServerOptions{BaseRouter: r})
})
```

## Data Flow

### Ingestion Flow (Verified)
```
SDK Client
    ↓ POST /v1/ingest
API Handler (validates, authenticates via api_key_hash)
    ↓
Ingest Service
    ↓ Claim batch (idempotency check)
    ↓ Begin transaction
Store Layer (upserts traces/spans, inserts events)
    ↓ Enqueue rollup job (same transaction)
    ↓ Commit
River Worker (async)
    ↓ Compute aggregates
    ↓ Update trace rollups with version check
```

### Query Flow
```
Web UI / API Client
    ↓ GET /api/traces?q=agent&status=FAILED
API Handler (auth, parse params)
    ↓
Store Layer (SQL with search/filters)
    ↓
PostgreSQL (uses GIN indexes for search)
    ↓
Mapper (translate DB fields to API fields)
    ↓
Response
```

---

# 4. Database Schema (Verified)

## Entity Relationship Diagram

```
┌─────────────┐
│  projects   │
│─────────────│
│ id (PK)     │
│ name        │
│ api_key     │
│ api_key_hash│◄─── SHA-256 for auth lookup
└─────────────┘
       │
       │ 1:N
       ▼
┌─────────────┐
│  sessions   │
│─────────────│
│ id (PK/UUID)│◄─── Internal ID only (no external session_id text)
│ project_id  │
│ name        │ (nullable)
│ user_id     │ (nullable)
│ metadata    │
└─────────────┘
       │
       │ 1:N
       ▼
┌─────────────┐
│   traces    │
│─────────────│
│ id (PK)     │
│ project_id  │
│ trace_id    │ (external TEXT)
│ session_id  │◄─── UUID FK to sessions.id
│ version     │◄─── For rollup race handling
│ total_spans │
│ total_tokens│◄─── Single aggregate (not in/out split)
│ total_cost  │
│ error_count │
└─────────────┘
       │
       │ 1:N
       ▼
┌─────────────┐
│    spans    │
│─────────────│
│ id (PK)     │
│ project_id  │
│ trace_id    │
│ span_id     │ (external TEXT)
│ parent_span │ (external TEXT)
└─────────────┘
       │
       │ 1:N
       ▼
┌─────────────┐
│ span_events │
│─────────────│
│ id (PK)     │
│ project_id  │
│ trace_id    │
│ span_id     │◄─── External span identifier (TEXT, not UUID FK)
│ idempotency │
└─────────────┘

┌────────────────┐
│ ingest_batches │
│────────────────│
│ id (PK)        │
│ project_id     │
│ batch_key      │ (UNIQUE per project)
│ status         │
└────────────────┘

┌────────────────┐
│   river_*      │◄─── River queue tables (auto-managed)
└────────────────┘
```

## Table Definitions (Verified)

### projects
```sql
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    api_key TEXT NOT NULL UNIQUE,
    api_key_hash TEXT NOT NULL,  -- SHA-256 hash for lookup
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### sessions
```sql
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    name TEXT,                    -- Optional display name
    user_id TEXT,                 -- Optional user identifier
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Note: No external session_id TEXT field
-- Sessions are referenced by their UUID id
```

### traces
```sql
CREATE TABLE traces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    trace_id TEXT NOT NULL,           -- External ID from SDK
    session_id UUID REFERENCES sessions(id),  -- UUID FK, not text
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    
    -- Timestamps
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    
    -- Payload (truncated)
    input JSONB,
    output JSONB,
    
    -- User context
    user_id TEXT,
    environment TEXT,
    release TEXT,
    tags TEXT[],
    metadata JSONB DEFAULT '{}',
    
    -- Rollup fields (computed by River worker)
    total_spans INTEGER DEFAULT 0,
    total_tokens BIGINT DEFAULT 0,      -- Single aggregate
    total_cost NUMERIC(20,10) DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    
    -- Version for rollup race handling
    version INTEGER DEFAULT 1,
    
    -- Search
    search_vector TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(user_id, '')), 'B')
    ) STORED,
    
    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE(project_id, trace_id)
);
```

### spans
```sql
CREATE TABLE spans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    trace_id UUID NOT NULL REFERENCES traces(id),
    span_id TEXT NOT NULL,            -- External ID from SDK
    parent_span_id TEXT,              -- External ID of parent (TEXT, not UUID)
    
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'custom',
    status TEXT NOT NULL DEFAULT 'running',
    level TEXT DEFAULT 'default',
    
    -- Timestamps
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    
    -- Payload (truncated)
    input JSONB,
    output JSONB,
    
    -- LLM-specific
    model TEXT,
    tokens_in INTEGER,
    tokens_out INTEGER,
    cost_usd NUMERIC(20,10),
    
    -- Metadata
    metadata JSONB DEFAULT '{}',
    
    -- Truncation tracking
    input_truncated BOOLEAN DEFAULT FALSE,
    output_truncated BOOLEAN DEFAULT FALSE,
    
    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE(project_id, trace_id, span_id)
);
```

### span_events
```sql
CREATE TABLE span_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    trace_id UUID NOT NULL REFERENCES traces(id),
    span_id TEXT,  -- External span identifier (TEXT, not UUID FK)
    
    event_type TEXT NOT NULL,
    level TEXT DEFAULT 'info',
    message TEXT,
    payload JSONB,
    event_ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Idempotency
    idempotency_key TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial unique index for idempotency
CREATE UNIQUE INDEX idx_events_idempotency 
    ON span_events(project_id, idempotency_key) 
    WHERE idempotency_key IS NOT NULL;
```

### ingest_batches
```sql
CREATE TABLE ingest_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    batch_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing',
    
    traces_count INTEGER DEFAULT 0,
    spans_count INTEGER DEFAULT 0,
    events_count INTEGER DEFAULT 0,
    
    error_message TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    
    UNIQUE(project_id, batch_key)
);
```

## Indexes (Implemented)

```sql
-- Traces
CREATE INDEX idx_traces_search ON traces USING GIN(search_vector);
CREATE INDEX idx_traces_project_status ON traces(project_id, status);
CREATE INDEX idx_traces_project_start_time ON traces(project_id, start_time DESC);
CREATE INDEX idx_traces_project_session ON traces(project_id, session_id);

-- Spans
CREATE INDEX idx_spans_trace ON spans(trace_id);
CREATE INDEX idx_spans_search ON spans USING GIN(to_tsvector('english', name));

-- Events
CREATE INDEX idx_events_trace ON span_events(trace_id);
```

## 4.1 Final Schema Corrections (Superseding Update)

The following schema details reflect the final Phase 3 fixes:

### sessions (final shape)
```sql
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    external_id TEXT NOT NULL,
    name TEXT,
    user_id TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_sessions_project_external
    ON sessions(project_id, external_id);
```

### traces rollup fields (final shape)
```sql
-- rollup fields
total_spans INTEGER DEFAULT 0,
total_tokens_in BIGINT NOT NULL DEFAULT 0,
total_tokens_out BIGINT NOT NULL DEFAULT 0,
total_cost NUMERIC(...),
error_count INTEGER DEFAULT 0
```

### migration note
- `000008_split_trace_tokens` introduces split token columns and drops old `total_tokens`.
- `000009_session_external_id` adds `sessions.external_id` and unique index.

---

# 5. API Surface (Verified)

## Base URL
- Development: `http://localhost:8080`
- Production: `https://api.continua.dev` (planned)

## Authentication

All `/api/*` endpoints (except health) require API key:
```
X-API-Key: ck_live_xxxxx
# or
Authorization: Bearer ck_live_xxxxx
```

Authentication uses SHA-256 hash lookup against `projects.api_key_hash`.

## Endpoints

### Health (Public)
```
GET /api/health

Response 200:
{
  "status": "ok",
  "version": "0.1.0"
}
```

### Traces

#### List Traces
```
GET /api/traces

Query Parameters:
- limit (int, default 20)
- offset (int, default 0)
- status (string: RUNNING, COMPLETED, FAILED)
- q (string, full-text search)
- start_time_from (datetime)
- start_time_to (datetime)
- user_id (string)
- session_id (string, UUID)
- has_errors (boolean)
- min_duration_ms (integer)

Response 200:
{
  "traces": [Trace],
  "total": 100,
  "limit": 20,
  "offset": 0
}
```

#### Get Trace
```
GET /api/traces/{id}

Response 200: Trace
Response 404: Not found
```

#### Get Trace Spans
```
GET /api/traces/{id}/spans

Response 200:
{
  "spans": [Span]
}
```

### Sessions

#### List Sessions
```
GET /api/sessions

Query Parameters:
- limit (int, default 20)
- offset (int, default 0)
# Note: user_id filter NOT implemented yet

Response 200:
{
  "sessions": [Session],
  "total": 50
}
```

#### Get Session
```
GET /api/sessions/{id}

Response 200: Session (includes trace_count)
```

### Ingestion

#### Ingest Batch
```
POST /v1/ingest
POST /v1/ingest?sync=true  # Wait for processing

Request Body:
{
  "batch_key": "optional-idempotency-key",
  "traces": [TraceInput],
  "spans": [SpanInput],
  "events": [EventInput]
}

Response 200 (sync=true):
{
  "status": "ok",
  "batch_key": "uuid",
  "traces_created": 1,
  "spans_created": 5,
  "events_created": 10
}

Response 202 (sync=false):
{
  "status": "accepted" | "duplicate",
  "batch_key": "uuid"
}
```

## API Types

### Trace (API Response)
```typescript
interface Trace {
  id: string;           // Internal UUID
  trace_id: string;     // External ID
  name: string;
  status: "RUNNING" | "COMPLETED" | "FAILED";
  start_time?: string;
  end_time?: string;
  input?: object;
  output?: object;
  user_id?: string;
  session_id?: string;  // UUID reference
  environment?: string;
  tags?: string[];
  metadata?: object;
  
  // Rollups (mapped from DB)
  total_tokens_in?: number;   // Derived in mapper
  total_tokens_out?: number;  // Derived in mapper
  total_cost_usd?: number;
  total_spans?: number;
  error_count?: number;
  
  created_at: string;
  updated_at: string;
}
```

### Span (API Response)
```typescript
interface Span {
  id: string;
  span_id: string;
  trace_id: string;
  parent_span_id?: string;
  name: string;
  kind: SpanKind;
  status: SpanStatus;
  level?: SpanLevel;
  start_time?: string;
  end_time?: string;
  input?: object;
  output?: object;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  cost_usd?: number;
  metadata?: object;
}
```

### Session (API Response)
```typescript
interface Session {
  id: string;           // UUID
  external_id: string;  // External session key (final Phase 3 fix)
  name?: string;        // Optional display name
  user_id?: string;
  trace_count?: number; // Computed
  metadata?: object;
  created_at: string;
  updated_at: string;
}
```

## 5.1 Final API Contract Corrections (Superseding Update)

### IngestTraceInput.session_id
- Final contract: plain `string` external key.
- Not UUID formatted at the API contract layer.

### IngestSpanInput total token policy
- `total_tokens` remains as deprecated compatibility field in OpenAPI.
- Server rejects `total_tokens`-only payloads if `prompt_tokens` and `completion_tokens` are absent.

### Session response contract
- Session list/detail includes:
  - `id` (internal UUID)
  - `external_id` (external key used by SDK/instrumentation)
  - existing fields (`name`, `user_id`, `trace_count`, `metadata`, timestamps)

---

# 6. Domain Types & Status Model

## Database Status Style (Internal)
Lowercase values stored in database:
- `running`
- `completed`
- `failed`
- `error` (variant, mapped to FAILED)
- `cancelled` (variant, handled in filters)

## API Status Style (External)
Uppercase normalized values:
- `RUNNING`
- `COMPLETED`
- `FAILED`

## Mapping Behavior

The mapper layer translates between DB and API representations:
- Status: lowercase → uppercase
- Tokens: `total_tokens` (DB) → `total_tokens_in`/`total_tokens_out` (API) via compatibility logic

## 6.1 Final Mapping & Token Policy Update (Superseding)

Final Phase 3 implementation changed token mapping semantics:
- Token mapping is now direct DB-to-API for traces:
  - `traces.total_tokens_in` -> `Trace.total_tokens_in`
  - `traces.total_tokens_out` -> `Trace.total_tokens_out`
- The old compatibility split from a single `total_tokens` value is removed.
- Session mapping now includes `Session.external_id` in all API response paths.
- Ingest validation explicitly rejects `total_tokens`-only spans.

## Span Kind
```
LLM        - Language model call
TOOL       - Tool/function execution
AGENT      - Sub-agent invocation
CHAIN      - Chain/pipeline step
RETRIEVAL  - Vector search / RAG
EMBEDDING  - Embedding generation
GENERATION - Text generation
CUSTOM     - User-defined (default)
```

## Span Level
```
DEBUG      - Verbose debugging info
DEFAULT    - Normal operation
WARNING    - Potential issue
ERROR      - Error occurred
```

## Event Type
```
LOG        - General log message
ERROR      - Error event
EXCEPTION  - Exception with stack trace
MESSAGE    - Chat message
METRIC     - Custom metric
CUSTOM     - User-defined
```

---

# 7. Implementation Status by Phase

## Phase 1: Foundation ✅ Completed

| Component | Status |
|-----------|--------|
| Monorepo structure | ✅ Done |
| PostgreSQL schema | ✅ Done |
| sqlc queries | ✅ Done |
| OpenAPI spec | ✅ Done |
| Code generation | ✅ Done |
| Domain types | ✅ Done |
| Store layer | ✅ Done |
| Ingest service | ✅ Done |
| Payload truncation | ✅ Done |
| Docker setup | ✅ Done |
| Kubernetes manifests | ✅ Done |

## Phase 2: End-to-End Usability ✅ Completed

| Component | Status |
|-----------|--------|
| Server bootstrap (Fx) | ✅ Done |
| `continua serve` | ✅ Done |
| Auth middleware | ✅ Done |
| Router composition | ✅ Done |
| Python SDK baseline | ✅ Done |
| Inline rollups | ✅ Done (replaced in P3) |
| Web UI - Traces | ✅ Done |
| Web UI - Detail | ✅ Done |
| API key prompt | ✅ Done |
| E2E workflow | ✅ Done |

## Phase 3: Reliability, Search & Sessions ✅ Completed

| Component | Status | Notes |
|-----------|--------|-------|
| Async rollups (River) | ✅ Done | Transactional enqueue, version-based reruns |
| Idempotency hardening | ✅ Done | COALESCE patterns, status protection |
| Search & filtering | ✅ Done | Full-text, multi-field filters |
| Query performance | ✅ Done | GIN indexes, project-scoped indexes |
| Sessions API | ✅ Done | Read/list/detail only |
| Sessions UI | ✅ Done | /sessions, /sessions/:id |
| Python SDK polish | ✅ Done | Exceptions, retry, session context |

### Phase 3 Final Fixes (Post-Audit + Runtime Verification) ✅ Completed

| Component | Status | Notes |
|-----------|--------|-------|
| Token rollup alignment | ✅ Done | Split DB columns (`total_tokens_in/out`), no mapper compatibility split |
| Session external IDs | ✅ Done | `sessions.external_id` + server-side get-or-create by external key |
| Ingest token contract enforcement | ✅ Done | Rejects `total_tokens`-only spans |
| OpenAPI + generated type alignment | ✅ Done | `session_id` external string, Session includes `external_id` |
| Python SDK contract alignment | ✅ Done | Prevents total-only token footgun; no `total_tokens` emission |
| Runtime startup hardening | ✅ Done | River startup context fix, Vite proxy target fix |

### Verification Results
```
go test ./internal/jobs      ✅ PASS
go test ./internal/ingest    ✅ PASS
go test ./internal/store     ✅ PASS
go test ./internal/api       ✅ PASS
go test ./internal/api/middleware  ✅ PASS
cd sdks/python && uv run pytest -q  ✅ 35 passed, 12 skipped
go test ./...                ✅ PASS
cd sdks/python && uv run pytest -q  ✅ 36 passed, 12 skipped
openspec validate fix-token-rollup-sessions --strict ✅ PASS
```

---

# 8. Phase 3 Feature Details (Verified)

## 8.1 Async Rollups via River ✅

**What Was Built:**
- River client Fx module (`internal/jobs/module.go`)
- Trace rollup worker (`internal/jobs/trace_rollup.go`)
- Transactional enqueue from ingest service
- Coalescing uniqueness strategy (dedupes pending jobs)
- Trace version-based rerun logic (handles concurrent updates)

**Flow:**
```
Ingest Service
    ↓ Span upserted
    ↓ Enqueue TraceRollupJob (same txn)
    ↓ Commit
    
River Worker (async)
    ↓ Read trace version
    ↓ Compute aggregates from spans
    ↓ Update trace with version check
    ↓ Retry if version mismatch
```

## 8.2 Idempotency Hardening ✅

**What Was Implemented:**

| Layer | Mechanism |
|-------|-----------|
| Batch | Claim via `ingest_batches` unique constraint |
| Event | Partial unique index on `idempotency_key` |
| Span | COALESCE upsert pattern |
| Status | Downgrade protection (failed is terminal) |
| Times | LEAST/GREATEST for out-of-order handling |

**Span Upsert Pattern:**
```sql
ON CONFLICT (project_id, trace_id, span_id) DO UPDATE SET
    output = COALESCE(EXCLUDED.output, spans.output),
    status = CASE 
        WHEN spans.status IN ('failed', 'error') THEN spans.status
        ELSE EXCLUDED.status 
    END,
    start_time = LEAST(EXCLUDED.start_time, spans.start_time),
    end_time = GREATEST(EXCLUDED.end_time, spans.end_time),
    metadata = spans.metadata || COALESCE(EXCLUDED.metadata, '{}')
```

## 8.3 Search & Filtering ✅

**What Was Implemented:**
- Full-text search via `search_vector` generated column
- GIN indexes on traces and spans
- Combined filter support

**Supported Filters:**
| Filter | Description |
|--------|-------------|
| `q` | Full-text search on trace name, user_id |
| `status` | RUNNING, COMPLETED, FAILED |
| `start_time_from` | Traces starting after datetime |
| `start_time_to` | Traces starting before datetime |
| `user_id` | Filter by user |
| `session_id` | Filter by session UUID |
| `has_errors` | Traces with error_count > 0 |
| `min_duration_ms` | Minimum duration filter |

**Search Behavior:**
- When `q` is present, results are ranked by relevance
- Filters combine with AND logic

## 8.4 Query Performance ✅

**Indexes Added:**
- `idx_traces_search` (GIN on search_vector)
- `idx_traces_project_status` (project_id, status)
- `idx_traces_project_start_time` (project_id, start_time DESC)
- `idx_traces_project_session` (project_id, session_id)
- `idx_spans_trace` (trace_id)
- `idx_spans_search` (GIN on name)

**Note:** Performance is validated via query planner checks, not fixed latency SLOs.

## 8.5 Sessions API + UI ✅

**What Was Implemented:**
- `GET /api/sessions` - List with pagination
- `GET /api/sessions/{id}` - Detail with trace_count
- `/sessions` - Web UI list page
- `/sessions/:id` - Web UI detail page

**Important Caveats:**
- Session creation is NOT a dedicated API endpoint
- Ingest references sessions by UUID; no auto-create in ingest
- Sessions API does NOT support `user_id` filter yet

## 8.6 Python SDK Polish ✅

**What Was Implemented:**

| Feature | Details |
|---------|---------|
| Custom exceptions | `AuthenticationError`, `RateLimitError`, `ValidationError`, `NetworkError` |
| Retry logic | Exponential backoff via tenacity |
| Session context | Context manager for session scoping |
| Span helpers | `set_llm_response()`, `set_tool_call()`, `log()` |
| Error messages | Clear, actionable error text |

**Usage:**
```python
from continua import Continua, trace, span, session
from continua.exceptions import AuthenticationError, RateLimitError

client = Continua(api_key="ck_live_...", endpoint="http://localhost:8080")

with session("my-session"):
    with trace(name="my-agent") as t:
        with span(name="llm-call", kind="LLM") as s:
            s.set_llm_response(
                model="gpt-4",
                messages=[...],
                response={...},
                tokens_in=100,
                tokens_out=50
            )
            s.set_cost(0.003)

try:
    client.flush()
except AuthenticationError:
    print("Invalid API key")
except RateLimitError as e:
    print(f"Retry after {e.retry_after}s")
```

## 8.7 Token Rollup Alignment Finalization ✅

**Final Implementation Scope:**
- DB migration to split trace token rollups
- SQL rollup query computes directional sums:
  - `SUM(prompt_tokens)` -> input rollup
  - `SUM(completion_tokens)` -> output rollup
- Trace update query stores both rollup directions directly
- Mapper now passes through directional fields from DB directly

**Why this matters:**
- Removes ambiguous token split hacks
- Aligns rollups with already-directional span data
- Prevents undercount/overcount artifacts

## 8.8 Session External ID Finalization ✅

**Final Implementation Scope:**
- `sessions.external_id` added with unique `(project_id, external_id)`
- new upsert path: `GetOrCreateSessionByExternalID`
- ingest treats all `session_id` strings as external keys
  - including UUID-looking strings
- session API responses include `external_id`

**Behavioral outcome:**
- SDK/client instrumentation can use human-readable session keys
- backend internally maps to UUID FK for storage integrity

## 8.9 Ingest Token Contract Tightening ✅

**Final rule:**
- Supported rollup inputs: `prompt_tokens` and/or `completion_tokens`
- Unsupported payload shape: only `total_tokens` with no directional tokens

**Result:**
- explicit validation failure instead of ambiguous partial acceptance
- deterministic rollup behavior and clearer SDK guidance

## 8.10 Runtime Verification Fixes ✅

### River startup context fix
- File: `internal/jobs/module.go`
- Change: worker client startup uses long-lived context (`context.Background()`)
- Impact: avoids notifier reconnect/error loops caused by short-lived startup context cancellation

### Web dev proxy alignment
- File: `web/vite.config.ts`
- Change:
  - `/api` proxy target -> `http://127.0.0.1:8080`
  - `/ws` proxy target -> `ws://127.0.0.1:8080`
- Impact: frontend dev proxy aligns with backend default port and works during local run flow

---

# 9. Known Gaps & Phase 4 Planning Inputs

## Known Gaps (Verified)

| Gap | Severity | Notes |
|-----|----------|-------|
| TypeScript SDK scaffold only | High | Not feature parity with Python |
| WebSocket backend incomplete | Medium | Contracts exist, pipeline not done |
| No session create API | Medium | Product decision needed |
| No `user_id` filter on sessions | Low | Easy to add |
| Config file not consumed | Low | Runtime uses env vars only |
| No benchmark suite | Low | Only planner validation |

## 9.1 Gap Status Reconciliation (Final Phase 3 Fixes)

The following previously-listed concerns are now resolved by final Phase 3 fixes:

1. **Token rollup compatibility ambiguity**: resolved via directional DB rollups and direct mapper mapping.
2. **Session external key ergonomics gap**: resolved via `external_id` storage and ingest auto-resolve/create.
3. **SDK/server token contract mismatch risk**: resolved with ingest validation + Python SDK guardrails.
4. **Local runtime startup instability (River notifier loop)**: resolved via worker startup context update.
5. **Frontend dev proxy port mismatch**: resolved via Vite proxy target alignment.

## Phase 4 Planning Inputs (Recommended)

### Priority 1: TypeScript SDK Parity
- Trace/span/session APIs matching Python
- Batching with configurable flush
- Retry with exponential backoff
- Custom exceptions
- Ergonomic helpers

### Priority 2: Real-time Updates
- Implement `/ws` endpoint aligned to existing contracts
- Publish events from ingest/rollup to connected clients
- UI integration for live trace updates

### Priority 3: Session Lifecycle
- Product decision: explicit create API vs auto-upsert in ingest
- Implement chosen approach
- Add `user_id` filter to sessions list

### Priority 4: Measurement & Hardening
- Reproducible benchmark harness
- Observable SLOs for ingest/rollup
- Migration drift checks in CI
- Config loader alignment with example file

---

# 10. Development Conventions

## Git Workflow

```bash
# Branch naming
feature/phase4-typescript-sdk
fix/session-user-filter
perf/benchmark-harness

# Conventional commits
feat: typescript sdk with batching
fix: add user_id filter to sessions api
perf: add benchmark suite
test: integration tests for websocket
docs: phase 4 report

# Never commit to main directly
# Always create PR with CodeRabbit review
```

## Code Generation

```bash
# After OpenAPI changes
make generate

# After SQL query changes
make generate-sqlc

# After protobuf changes (if used)
make generate-proto
```

## Testing Policy

- **Never bypass, skip, or remove failing tests**
- **Never disable CI checks to make builds pass**
- **Always perform root cause analysis (RCA) first**
- **Fix the actual issue, not the symptom**

## Never Edit Manually

- `*_gen.go` files (oapi-codegen output)
- `db/gen/*` files (sqlc output)
- `web/src/api/types.ts` (generated from OpenAPI)

---

# 11. Key Architectural Decisions

## ADR-001: SDK-First over Proxy Mode

**Decision**: SDK-first integration (decorators, context managers).

**Rationale**:
- Better semantic context (knows trace vs. span)
- Works with any LLM provider
- No network hop overhead

## ADR-002: PostgreSQL as Primary Store

**Decision**: PostgreSQL for all data.

**Rationale**:
- Simpler operations
- ACID transactions for ingest
- Full-text search built-in
- River queue native

## ADR-003: Contract-First API Design

**Decision**: OpenAPI spec is source of truth.

**Rationale**:
- Single source of truth
- Generated types prevent drift
- Documentation always accurate

## ADR-004: River for Job Queue

**Decision**: River (PostgreSQL-native queue).

**Rationale**:
- No new infrastructure
- Transactional enqueue
- Built-in retry/backoff

## ADR-005: SHA-256 for API Key Auth

**Decision**: Hash API keys for lookup.

**Rationale**:
- Don't store plaintext keys
- Fast hash comparison
- Standard security practice

## ADR-006: Router Composition for Auth

**Decision**: Chi router composition (not middleware bypass).

**Rationale**:
- Clear public/protected separation
- No path hardcoding in middleware
- Easier to extend

## ADR-007: Trace Version for Rollup Safety

**Decision**: Version field on traces for concurrent rollup handling.

**Rationale**:
- Multiple rollup jobs can race
- Version check prevents stale writes
- Retry on version mismatch

---

# 12. Reference Implementations Studied

| Project | What We Learned | What We Adopted |
|---------|-----------------|-----------------|
| Langfuse | Trace/span model, token tracking | Similar data model |
| Helicone | Proxy patterns | Chose SDK-first instead |
| LiteLLM | Multi-provider abstraction | Potential integration target |
| OpenLLMetry | OTel conventions | Custom model for control |
| Arize Phoenix | Evaluation workflows | Future inspiration |
| Pydantic Logfire | Decorator patterns | Similar SDK design |

---

# 13. Appendix: File Structure

## Repository Layout (Verified)

```
cmd/continua/
├── main.go               # Entrypoint
├── serve.go              # Fx app
└── version.go

internal/
├── api/
│   ├── module.go         # Fx module
│   ├── router.go         # Chi router
│   ├── server.go         # HTTP handlers
│   ├── server_gen.go     # Generated (don't edit)
│   ├── mapper.go         # Domain → API
│   └── middleware/
│       └── auth.go       # API key middleware (SHA-256)
│
├── config/
│   ├── module.go
│   └── config.go         # Env var loader
│
├── store/
│   ├── module.go
│   ├── pool.go           # pgxpool
│   ├── store.go
│   ├── traces.go
│   ├── spans.go
│   ├── events.go
│   ├── sessions.go
│   ├── projects.go
│   ├── batches.go
│   └── rollups.go
│
├── ingest/
│   ├── module.go
│   ├── service.go        # Batch processing + job enqueue
│   ├── dto.go
│   └── validation.go
│
├── jobs/
│   ├── module.go         # River Fx module
│   └── trace_rollup.go   # Rollup worker
│
├── domain/
│   ├── trace.go
│   ├── span.go
│   ├── event.go
│   └── session.go
│
└── ws/                   # Placeholder for WebSocket

pkg/
└── truncation/
    ├── truncation.go
    └── truncation_test.go

contracts/
├── openapi/
│   └── openapi.yaml      # Source of truth
└── websocket/            # WebSocket contracts (not implemented)

db/
├── platform/
│   ├── migrations/
│   │   └── postgres/
│   │       ├── 0001_initial_schema.up.sql
│   │       ├── XXXX_add_search_indexes.up.sql
│   │       └── ...
│   └── queries/
│       ├── traces.sql
│       ├── spans.sql
│       └── ...
└── gen/                  # Generated (don't edit)

sdks/
├── python/
│   ├── pyproject.toml
│   ├── src/
│   │   └── continua/
│   │       ├── __init__.py
│   │       ├── client.py
│   │       ├── trace.py
│   │       ├── span.py
│   │       ├── batch.py
│   │       ├── types.py
│   │       └── exceptions.py
│   └── tests/
│
└── typescript/           # Scaffold only
    └── ...

web/
├── package.json
├── vite.config.ts
├── src/
│   ├── App.tsx
│   ├── main.tsx
│   ├── pages/
│   │   ├── TracesPage.tsx
│   │   ├── TraceDetailPage.tsx
│   │   ├── SessionsPage.tsx
│   │   └── SessionDetailPage.tsx
│   ├── components/
│   │   ├── ApiKeyPrompt.tsx
│   │   ├── SpanTree.tsx
│   │   ├── SpanDetail.tsx
│   │   └── StatusBadge.tsx
│   ├── api/
│   │   └── client.ts
│   └── utils/
│       └── format.ts

deploy/
├── docker-compose/
│   ├── docker-compose.yml
│   ├── docker-compose.dev.yml
│   └── docker-compose.test.yml
└── kubernetes/
    ├── kustomize/
    └── helm/

.github/
└── workflows/
    ├── ci.yml
    └── e2e.yml

docs/
├── phase2/
│   └── report.md
└── phase3/
    ├── report.md
    └── report_phase3_fixes_addendum.md

Makefile
go.mod
go.sum
CLAUDE.md
README.md
```

---

# Quick Reference

## Common Commands

```bash
# Development
make dev              # Start dev server
make test             # Run all tests
make lint             # Run linters
make generate         # Regenerate code
make ci               # Full CI check

# Database
make migrate-up       # Apply migrations
make migrate-down     # Rollback migrations
make db-reset         # Reset database

# SDK
cd sdks/python && uv run pytest  # Test Python SDK

# Web
cd web && npm run dev  # Start UI dev server
```

## Environment Variables

```bash
# Required
DATABASE_URL=postgres://user:pass@localhost:5432/continua

# Optional
HOST=0.0.0.0          # Server host (default)
PORT=8080             # Server port (default)
LOG_LEVEL=info        # Logging level
```

## API Quick Test

```bash
# Health check
curl http://localhost:8080/api/health

# List traces (requires auth)
curl -H "X-API-Key: ck_live_xxx" http://localhost:8080/api/traces

# Search traces
curl -H "X-API-Key: ck_live_xxx" \
  "http://localhost:8080/api/traces?q=agent&status=FAILED&has_errors=true"

# Ingest
curl -X POST \
  -H "X-API-Key: ck_live_xxx" \
  -H "Content-Type: application/json" \
  -d '{"traces":[...],"spans":[...]}' \
  http://localhost:8080/v1/ingest
```

---

# Document Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | Jan 2026 | Initial comprehensive report |
| 1.1 | Jan 2026 | Updated Phase 3 as completed |
| 2.0 | Mar 2026 | Codex audit verification pass; corrected schema/API details |
| 2.1 | Mar 2026 | Added final Phase 3 fixes: split trace token rollups, session external IDs, ingest token policy enforcement, Python SDK contract alignment, and runtime startup/proxy fixes |

---

# 14. Phase 3 Final Fixes (Superseding Update)

This section is intentionally redundant and explicit to support agent handoff with minimal ambiguity.
It captures exactly what was finalized after the prior report text and should be treated as authoritative where conflicts exist.

## 14.1 Implemented Fix Set

### A) Token Rollup Realignment
- Added trace-level directional columns (`total_tokens_in`, `total_tokens_out`)
- Backfilled from span-level directional token fields
- Removed old single `total_tokens` trace column
- Updated rollup SQL and update SQL paths
- Updated store/domain/mapper to consume directional rollups directly

### B) Session External ID Support
- Added `sessions.external_id` (not null)
- Added unique index `(project_id, external_id)`
- Added upsert query for get-or-create by external key
- Updated ingest service to always treat incoming `session_id` as external key
- Added `external_id` to session API mapping and response schema

### C) Ingest Contract Tightening
- Enforced validation rule:
  - reject spans with `total_tokens` but no `prompt_tokens`/`completion_tokens`
- Updated OpenAPI description for deprecated `total_tokens` semantics

### D) SDK Contract Alignment
- Python session helper no longer assumes UUID generation for session context
- Python span builder no longer emits `total_tokens`
- Python span API raises on unsupported `set_tokens(total=...)`-only path
- Added/updated tests to reflect new contract

### E) Runtime Fixes from Real Run
- River worker startup context switched to long-lived context to prevent notifier reconnect loops
- Vite proxy target corrected to backend default port (8080)

## 14.2 Validation Evidence (Final)

### Codegen / Contract
- `make generate` completed successfully after OpenAPI/schema updates

### Go Tests
- `go test ./...` passed after fixes
- target package checks also passed:
  - `internal/jobs`
  - `internal/ingest`
  - `internal/store`
  - `internal/api`

### Python Tests
- `cd sdks/python && uv run pytest -q` passed (36 passed, 12 skipped)

### OpenSpec
- `openspec validate fix-token-rollup-sessions --strict` passed

### Runtime Checks
- backend start + health endpoint check passed (`/api/health` 200)
- frontend dev server + API proxy behavior verified after proxy alignment

---

*This document is the verified source of truth for Continua development through Phase 3. Use it to bootstrap Phase 4 planning with confidence that implementation details are accurate.*
