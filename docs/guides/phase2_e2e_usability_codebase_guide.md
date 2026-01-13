# Phase 2: End-to-End Usability Codebase Guide

This guide documents what was implemented for Continua Phase 2, how the platform is now runnable end-to-end, and how requests flow through the codebase.

---

## 1. Overview

### What Phase 2 Aimed to Achieve

Phase 2 ("Enable End-to-End Platform Usability") aimed to wire together existing components to make Continua runnable and usable end-to-end. Before Phase 2, Continua had a complete data layer (store, ingest service, schema) but the platform wasn't usable because: the server didn't start (Fx DI incomplete), authentication wasn't enforced, there was no SDK to send data, no UI to view data, and rollups weren't computed.

### What Was Actually Added/Changed

Phase 2 added:
- **Server Bootstrap**: Fx modules for DI, config loading from env vars, HTTP server lifecycle with graceful shutdown
- **Authentication**: API key middleware with SHA-256 hashing, project context injection, multi-tenant scoping
- **Python SDK**: HTTP client with httpx, trace/span decorators, automatic batching with background flush
- **Trace Rollups**: Inline computation after ingest (total_spans, total_tokens, total_cost, error_count)
- **Web UI**: Traces list page with pagination, trace detail page with span tree visualization
- **Router Composition**: Health endpoint routed directly (public), OpenAPI routes under auth middleware
- **SPA Handler**: Embedded web UI served at "/" for production builds

### OpenSpec Documents

| Document | Location | Status |
|----------|----------|--------|
| Proposal | `openspec/changes/enable-e2e-usability/proposal.md` | Implemented |
| Design | `openspec/changes/enable-e2e-usability/design.md` | Implemented |
| Tasks | `openspec/changes/enable-e2e-usability/tasks.md` | Implemented |
| Spec: Server Bootstrap | `openspec/changes/enable-e2e-usability/specs/server-bootstrap/spec.md` | Implemented |
| Spec: Authentication | `openspec/changes/enable-e2e-usability/specs/authentication/spec.md` | Implemented |
| Spec: Python SDK | `openspec/changes/enable-e2e-usability/specs/python-sdk/spec.md` | Implemented |
| Spec: Trace Rollups | `openspec/changes/enable-e2e-usability/specs/trace-rollups/spec.md` | Implemented |
| Spec: Web UI | `openspec/changes/enable-e2e-usability/specs/web-ui/spec.md` | Implemented |
| Spec: OpenAPI Extensions | `openspec/changes/enable-e2e-usability/specs/openapi-extensions/spec.md` | Implemented |

### Related Architecture Docs

- `docs/CONTINUA_ARCHITECTURE_PLAN_v1.md` - Overall architecture
- `docs/CONTINUA_DESIGN_DECISIONS_v1.md` - Design decisions
- `docs/INGESTION_FLOW.mermaid.md` - Ingestion flow diagram

---

## 2. High-Level Architecture

### Components

```mermaid
graph TB
    subgraph CLI["CLI Layer"]
        MAIN[cmd/continua/main.go]
    end

    subgraph DI["Dependency Injection (Uber Fx)"]
        CONFIG[config.Module]
        STORE_MOD[store.Module]
        INGEST_MOD[ingest.Module]
        API_MOD[api.Module]
    end

    subgraph HTTP["HTTP Layer"]
        ROUTER[api/router.go]
        HEALTH[HealthCheck - Public]
        AUTH[APIKeyAuth Middleware]
        HANDLERS[OpenAPI Handlers]
        SPA[web.Handler - SPA]
    end

    subgraph Business["Business Logic"]
        SERVER[api/server.go]
        INGEST[ingest/service.go]
        ROLLUPS[store/rollups.go]
    end

    subgraph Data["Data Layer"]
        POOL[store/pool.go]
        STORE[store/store.go]
        SQLC[db/gen/go/platform]
    end

    subgraph External["External"]
        DB[(PostgreSQL)]
        SDK[Python SDK]
        UI[Web Browser]
    end

    MAIN --> CONFIG
    MAIN --> STORE_MOD
    MAIN --> INGEST_MOD
    MAIN --> API_MOD

    CONFIG --> STORE_MOD
    STORE_MOD --> INGEST_MOD
    STORE_MOD --> API_MOD

    API_MOD --> ROUTER
    ROUTER --> HEALTH
    ROUTER --> AUTH
    AUTH --> HANDLERS
    ROUTER --> SPA

    HANDLERS --> SERVER
    SERVER --> INGEST
    INGEST --> ROLLUPS
    INGEST --> STORE
    ROLLUPS --> STORE
    STORE --> POOL
    POOL --> DB

    SDK -->|POST /v1/ingest| AUTH
    UI -->|GET /api/traces| AUTH
    UI -->|GET /| SPA
```

### Key Design Rules Implemented

#### Contract-First Development
- OpenAPI spec at `contracts/openapi/openapi.yaml` is source of truth
- Go types generated via `oapi-codegen` to `internal/api/server_gen.go`
- TypeScript types generated to `contracts/generated/typescript/api.ts`
- Python types generated to `sdks/python/src/continua/types.py`

#### Auth Rules (Public vs Protected)
- `/api/health` - Routed directly in Chi, NO auth middleware (public for load balancers)
- All OpenAPI routes - Protected by `APIKeyAuth` middleware group
- No path-based bypass logic in middleware (clean composition pattern)

#### Project ID Scoping / Multi-Tenancy
- Every data endpoint filters by `project_id` from auth context
- `GetTrace` / `ListSpansByTrace` verify trace belongs to project (return 404 if not)
- Ingest associates all data with authenticated project

#### Rollup Consistency Model
- **Inline computation**: Rollups computed at end of ingest transaction
- Non-blocking on failure: log warning, don't abort transaction
- Fields updated: `total_spans`, `total_tokens`, `total_cost`, `error_count`

#### Test Environment
- Docker Compose for PostgreSQL: `deploy/docker-compose/docker-compose.dev.yml`
- Default credentials: `continua:continua`
- Default API key for testing: `test-api-key-12345` (or `default` after migration)

---

## 3. Directory & File Map (Phase 2 Relevant)

### Modified Files

| File | Purpose |
|------|---------|
| `cmd/continua/main.go` | CLI entrypoint with `serve` command, Fx app wiring |
| `contracts/openapi/openapi.yaml` | Added `span_id`, `error_count`, `input`, `output` fields |
| `internal/api/mapper.go` | Maps DB types to API types including new fields |
| `internal/api/server.go` | Handlers with project scoping, session_id filter |
| `internal/ingest/service.go` | Ingest with inline rollup computation |
| `web/src/App.tsx` | Router setup for traces pages |
| `web/vite.config.ts` | Proxy to backend on port 8081 |

### New Files - Server Bootstrap

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `internal/config/config.go` | Env-based config loading | `Config`, `ServerConfig`, `DatabaseConfig`, `Load()` |
| `internal/config/module.go` | Fx module | `var Module = fx.Provide(Load)` |
| `internal/store/pool.go` | Connection pool creation | `NewPool(ctx, cfg)`, `PoolConfig` |
| `internal/store/module.go` | Fx module | `var Module = fx.Module("store", ...)` |
| `internal/ingest/module.go` | Fx module | `var Module = fx.Provide(NewService)` |
| `internal/api/module.go` | Fx module | `var Module = fx.Module("api", ...)` |
| `internal/api/router.go` | Router assembly | `NewRouter(server, store) http.Handler` |

### New Files - Authentication

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `internal/api/middleware/auth.go` | Auth middleware | `APIKeyAuth(store)`, `GetProjectID(ctx)`, `hashAPIKey()` |

### New Files - Rollups

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `db/platform/queries/rollups.sql` | Rollup aggregation query | `ComputeTraceRollups` |
| `db/gen/go/platform/rollups.sql.go` | Generated query code | Auto-generated |
| `internal/store/rollups.go` | Rollup methods | `ComputeAndUpdateTraceRollups()`, `ComputeAndUpdateTraceRollupsTx()` |

### New Files - Python SDK

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `sdks/python/src/continua/client.py` | Main client | `Continua`, `init()`, `get_instance()`, `flush()`, `shutdown()` |
| `sdks/python/src/continua/batch.py` | Batching | `BatchQueue`, `add_trace()`, `add_span()`, background flush thread |
| `sdks/python/src/continua/trace.py` | Trace context | `TraceContext`, `@trace` decorator, `get_current_trace()` |
| `sdks/python/src/continua/span.py` | Span context | `SpanContext`, `span()` context manager |
| `sdks/python/tests/test_*.py` | Unit tests | Trace, span, batch tests |
| `sdks/python/examples/e2e_demo.py` | Demo script | End-to-end usage example |

### New Files - Web UI

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `web/src/api/client.ts` | API client | `fetchAPI()`, `getApiKey()`, `setApiKey()`, interfaces |
| `web/src/pages/TracesPage.tsx` | Traces list | Pagination with limit/offset |
| `web/src/pages/TraceDetailPage.tsx` | Trace detail | Two-panel layout, span selection |
| `web/src/components/SpanTree.tsx` | Span tree | Recursive tree, uses `span_id` for parent-child |
| `web/src/components/SpanDetail.tsx` | Span detail panel | JSON viewer for input/output |
| `web/src/components/ApiKeyPrompt.tsx` | API key input | localStorage storage |
| `web/src/components/StatusBadge.tsx` | Status badge | Color-coded by status |
| `web/src/utils/format.ts` | Formatting utils | `formatDuration()`, `formatTokens()`, `formatCost()` |

### New Files - Migrations

| File | Purpose |
|------|---------|
| `db/platform/migrations/postgres/000002_fix_default_api_key_hash.up.sql` | Fix API key hash in seed |
| `db/platform/migrations/postgres/000002_fix_default_api_key_hash.down.sql` | Revert migration |

---

## 4. Public Surface Area

### 4.1 CLI

#### Commands

```bash
# Start the server
continua serve

# Show version
continua version
```

#### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `HOST` | No | `0.0.0.0` | Server bind host |
| `PORT` | No | `8080` | Server bind port |

#### How to Run Locally

```bash
# 1. Start PostgreSQL
make dev

# 2. Run migrations
migrate -path db/platform/migrations/postgres \
  -database "postgres://continua:continua@localhost:5432/continua?sslmode=disable" up

# 3. Start server
DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable" \
PORT=8081 go run ./cmd/continua serve

# Or with hot reload:
make dev-server
```

### 4.2 HTTP API

#### Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/health` | No | Health check |
| POST | `/v1/ingest` | Yes | Batch ingestion |
| GET | `/api/traces` | Yes | List traces (limit/offset pagination) |
| GET | `/api/traces/{id}` | Yes | Get trace by ID |
| GET | `/api/traces/{id}/spans` | Yes | List spans for trace |
| GET | `/api/sessions` | Yes | List sessions |

#### Authentication

```bash
# Header options:
X-API-Key: your-api-key
# OR
Authorization: Bearer your-api-key
```

#### Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 202 | Accepted (async ingest) |
| 400 | Validation error |
| 401 | Missing/invalid API key |
| 404 | Not found (or forbidden due to multi-tenancy) |
| 413 | Request body too large (5MB limit) |
| 500 | Internal error |

#### Request Size Limits

- **Ingest endpoint**: 5MB maximum body size
- Enforced via `http.MaxBytesReader` in `internal/api/server.go:63`

### 4.3 UI

#### Routes

| Path | Component | Description |
|------|-----------|-------------|
| `/` | - | Redirects to /traces |
| `/traces` | `TracesPage` | Paginated traces list |
| `/traces/:id` | `TraceDetailPage` | Trace detail with span tree |

#### API Key Storage

- Stored in `localStorage` under key `continua_api_key`
- Prompted on first visit via `ApiKeyPrompt` component
- Sent in `X-API-Key` header on all API calls

### 4.4 SDK (Python)

#### Installation

```bash
cd sdks/python
pip install -e .
# Or with uv:
uv sync --all-extras
```

#### Public API

```python
from continua import Continua, trace, span

# Initialize (once at startup)
Continua.init(api_key="your-key", endpoint="http://localhost:8081")

# Use decorators
@trace
def my_agent():
    with span("llm_call", kind="llm") as s:
        s.set_model("gpt-4")
        s.set_tokens(prompt=100, completion=50)
        s.set_output({"result": "..."})

# Graceful shutdown
Continua.get_instance().shutdown()
```

#### Key Methods

| Method | Description |
|--------|-------------|
| `Continua.init(api_key, endpoint, ...)` | Initialize global client |
| `Continua.get_instance()` | Get singleton instance |
| `client.flush()` | Immediately send pending data |
| `client.shutdown()` | Flush and close connections |
| `@trace` | Decorator to create a trace |
| `span(name, kind)` | Context manager for spans |

---

## 5. End-to-End Code Flow

### 5.1 Server Startup Flow

```
1. main() [cmd/continua/main.go:27]
   └─> rootCmd.AddCommand(serveCmd())
   └─> rootCmd.Execute()

2. serveCmd().RunE [cmd/continua/main.go:47-49]
   └─> runServer()

3. runServer() [cmd/continua/main.go:53-73]
   └─> fx.New(
         config.Module,      // Provides *config.Config
         store.Module,       // Provides *pgxpool.Pool, *store.Store
         ingest.Module,      // Provides *ingest.Service
         api.Module,         // Provides *api.Server, http.Handler
         fx.Invoke(startHTTPServer),
       )
   └─> app.Run()

4. Fx Dependency Resolution Order:
   a. config.Load() [internal/config/config.go:34-59]
      └─> Reads DATABASE_URL, HOST, PORT from env
      └─> Returns *Config

   b. store.NewPool(ctx, cfg) [internal/store/pool.go:33-48]
      └─> Parses DATABASE_URL
      └─> Creates pgxpool.Pool with defaults

   c. store.New(pool) [internal/store/store.go]
      └─> Wraps pool with sqlc queries

   d. ingest.NewService(store) [internal/ingest/service.go:26-28]
      └─> Returns *ingest.Service

   e. api.NewServer(store) [internal/api/server.go:34-38]
      └─> Creates API handler (also creates ingest.Service internally)

   f. api.NewRouter(server, store) [internal/api/router.go:17-45]
      └─> Creates Chi router
      └─> Mounts /api/health (public)
      └─> Mounts OpenAPI handlers under auth group
      └─> Mounts SPA at /*

5. startHTTPServer(lc, cfg, handler) [cmd/continua/main.go:76-110]
   └─> Creates http.Server with timeouts
   └─> Registers Fx lifecycle hooks:
       OnStart: go server.ListenAndServe()
       OnStop: server.Shutdown(ctx)
```

### 5.2 Auth Flow

```
Request: GET /api/traces
         Header: X-API-Key: test-api-key-12345

1. Chi Router receives request
   └─> Matches /api/traces (protected group)

2. middleware.APIKeyAuth(store) [internal/api/middleware/auth.go:26-54]
   └─> extractAPIKey(r) [line 65-78]
       └─> Checks X-API-Key header first
       └─> Falls back to Authorization: Bearer
   └─> hashAPIKey(apiKey) [line 81-84]
       └─> SHA-256 hash of raw key
   └─> store.GetProjectByAPIKey(ctx, keyHash)
       └─> Queries projects table by api_key_hash
   └─> If not found: writeAuthError(401, "invalid_api_key")
   └─> If found: context.WithValue(r.Context(), ProjectIDKey, project.ID)
   └─> next.ServeHTTP(w, r.WithContext(ctx))

3. Handler extracts project ID:
   projectID, ok := middleware.GetProjectID(r.Context())
   └─> Returns uuid.UUID from context

Health endpoint bypass:
- /api/health is routed BEFORE the auth middleware group
- r.Get("/api/health", server.HealthCheck) [router.go:27]
- No middleware applied to this route
```

### 5.3 Ingest-to-Read Flow

#### Ingest Flow

```
Request: POST /v1/ingest
         Body: {"batch_key": "...", "traces": [...], "spans": [...]}

1. server.Ingest(w, r, params) [internal/api/server.go:55-144]
   └─> Check content length < 5MB [line 57-60]
   └─> Parse JSON body
   └─> Get project ID from context
   └─> convertToServiceRequest(req)

2. ingestService.Ingest(ctx, projectID, svcReq) [internal/ingest/service.go:45-189]
   a. Validate batch [line 51-54]
   b. Begin transaction [line 57-61]
   c. Claim batch (idempotency) [line 64-74]
      └─> If duplicate: return {status: "duplicate"}
   d. Process traces [line 81-89]
      └─> upsertTrace() for each
      └─> Build traceMap: external_id -> internal_uuid
   e. Process spans [line 92-113]
      └─> Resolve trace UUID from map or DB
      └─> upsertSpan() for each
   f. Process events [line 116-141]
      └─> insertEvent() with idempotency check
   g. Compute rollups [line 143-150]
      └─> For each affected trace:
          store.ComputeAndUpdateTraceRollupsTx(ctx, tx, traceUUID)
          └─> Aggregates from spans: COUNT, SUM tokens, SUM cost, COUNT errors
          └─> Updates trace record
   h. Update batch status [line 162-170]
   i. Commit transaction [line 176-178]
   j. Return response [line 180-188]
```

#### Read Flow

```
Request: GET /api/traces?limit=20&offset=0

1. server.ListTraces(w, r, params) [internal/api/server.go:146-197]
   └─> Get project ID from context
   └─> If params.SessionId provided:
       store.ListTracesBySession(ctx, projectID, sessionID)
   └─> Else:
       store.ListTraces(ctx, projectID, limit, offset)
   └─> store.CountTraces(ctx, projectID)
   └─> Map to API types via traceToAPI()
   └─> Return TraceList{traces, total}

Request: GET /api/traces/{id}/spans

1. server.ListSpansByTrace(w, r, id) [internal/api/server.go:218-257]
   └─> Get project ID from context
   └─> store.GetTrace(ctx, id)
   └─> Verify trace.ProjectID == projectID (else 404)
   └─> store.ListSpansByTrace(ctx, id)
   └─> Map to API types via spanToAPI()
   └─> Return SpanList{spans}
```

### 5.4 Rollups Flow

```
Triggered: At end of ingest transaction (inline)

1. After upserting spans, collect affected trace UUIDs
   [internal/ingest/service.go:143-150]

2. For each trace UUID:
   store.ComputeAndUpdateTraceRollupsTx(ctx, tx, traceUUID)
   [internal/store/rollups.go:50-73]

3. ComputeTraceRollups query [db/platform/queries/rollups.sql:1-9]
   SELECT
     COUNT(*)::int AS total_spans,
     COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
     COALESCE(SUM(total_cost), 0) AS total_cost,
     COUNT(*) FILTER (WHERE status IN ('failed', 'error'))::int AS error_count
   FROM spans WHERE trace_id = $1;

4. UpdateTraceRollups query [db/platform/queries/traces.sql:73-86]
   UPDATE traces SET
     total_spans = $2,
     total_tokens = $3,
     total_cost = $4,
     error_count = $5,
     duration_ms = CASE WHEN end_time IS NOT NULL AND start_time IS NOT NULL
       THEN EXTRACT(EPOCH FROM (end_time - start_time)) * 1000
       ELSE duration_ms END,
     updated_at = NOW()
   WHERE id = $1;

5. Error handling:
   - If rollup fails, log warning but continue
   - Ingest transaction is NOT aborted
   - Rollups are eventually consistent
```

---

## 6. Data Model + Multi-Tenancy

### Project ID Scoping

Every entity belongs to a project. The `project_id` column exists on:
- `traces`
- `spans`
- `span_events`
- `sessions`
- `ingest_batches`

All queries filter by `project_id`:
```sql
-- Example: ListTraces
SELECT * FROM traces WHERE project_id = $1 ORDER BY ... LIMIT $2 OFFSET $3;

-- Example: Ingest scoping
INSERT INTO traces (project_id, ...) VALUES ($1, ...);
```

### External vs Internal ID Mapping

| Entity | External ID (SDK) | Internal ID (DB) | Relationship |
|--------|-------------------|------------------|--------------|
| Trace | `trace_id` (TEXT) | `id` (UUID) | `UNIQUE(project_id, trace_id)` |
| Span | `span_id` (TEXT) | `id` (UUID) | `UNIQUE(trace_id, span_id)` |
| Parent Span | `parent_span_id` (TEXT) | - | References external `span_id` |

**Key insight**: `parent_span_id` is stored as external TEXT, not a foreign key. This allows:
- Out-of-order span arrival (parent may come after child)
- SDK uses simple string IDs
- UI builds tree using `span_id` field in API response

### API Response Fields

The API returns BOTH IDs for spans:
```json
{
  "id": "internal-uuid",       // Internal UUID (for API routes)
  "span_id": "external-string", // External ID (for tree building)
  "parent_span_id": "external-string" // References span_id
}
```

---

## 7. Database Read/Write Walkthrough

### Projects Table

| Operation | When | SQL Pattern |
|-----------|------|-------------|
| Read by API key | Auth middleware | `SELECT * FROM projects WHERE api_key_hash = $1` |

**Index**: `idx_projects_api_key ON projects(api_key_hash)`

**Seed data** (migration 0001):
```sql
INSERT INTO projects (id, name, api_key_hash)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Project', 'default');
```

**Fix** (migration 0002):
```sql
UPDATE projects SET api_key_hash = '<sha256 hash>' WHERE id = '...' AND api_key_hash = 'default';
```

### Traces Table

| Operation | When | SQL Pattern |
|-----------|------|-------------|
| Upsert | Ingest | `INSERT ... ON CONFLICT (project_id, trace_id) DO UPDATE` |
| List | GET /api/traces | `SELECT * FROM traces WHERE project_id = $1 ORDER BY ... LIMIT OFFSET` |
| Get | GET /api/traces/{id} | `SELECT * FROM traces WHERE id = $1` + project check |
| Rollup update | After ingest | `UPDATE traces SET total_spans = ..., total_tokens = ..., total_cost = ..., error_count = ...` |

**Unique constraint**: `UNIQUE(project_id, trace_id)` - ensures one trace per external ID per project

**Upsert semantics**:
- NULL values don't overwrite existing (COALESCE pattern)
- Status protection: 'failed'/'error' never downgraded
- Metadata is shallow-merged (||)

### Spans Table

| Operation | When | SQL Pattern |
|-----------|------|-------------|
| Upsert | Ingest | `INSERT ... ON CONFLICT (trace_id, span_id) DO UPDATE` |
| List by trace | GET /api/traces/{id}/spans | `SELECT * FROM spans WHERE trace_id = $1` |

**Unique constraint**: `UNIQUE(trace_id, span_id)`

**Key columns for tree building**:
- `span_id` (TEXT) - external identifier
- `parent_span_id` (TEXT) - external parent reference (NO FK constraint)

### Ingest Batches Table

| Operation | When | SQL Pattern |
|-----------|------|-------------|
| Claim | Start of ingest | `INSERT INTO ingest_batches ... ON CONFLICT (project_id, batch_key) DO NOTHING` |
| Update status | End of ingest | `UPDATE ingest_batches SET status = 'accepted', ...` |

**Unique constraint**: `UNIQUE(project_id, batch_key)` - idempotency key

**Idempotency flow**:
1. Try INSERT with batch_key
2. If conflict, return existing batch (duplicate)
3. If insert succeeds, continue processing

---

## 8. Worked Example

### Scenario: Ingest a trace with nested spans, then view in UI

#### Step 1: Start Server

```bash
# Terminal 1: Start DB
make dev

# Terminal 2: Run migrations
migrate -path db/platform/migrations/postgres \
  -database "postgres://continua:continua@localhost:5432/continua?sslmode=disable" up

# Terminal 3: Start server
DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable" \
PORT=8081 make dev-server
```

#### Step 2: Get API Key

The default project has API key `test-api-key-12345` (SHA-256 hash stored in DB).

```bash
# Verify auth works
curl -s http://localhost:8081/api/health
# {"status":"ok","version":"0.1.0"}

curl -s -H "X-API-Key: test-api-key-12345" http://localhost:8081/api/traces
# {"traces":[],"total":0}
```

#### Step 3: Ingest a Trace with Spans

```bash
curl -X POST http://localhost:8081/v1/ingest \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-api-key-12345" \
  -d '{
    "batch_key": "demo-batch-001",
    "traces": [{
      "trace_id": "my-agent-run-1",
      "name": "research_agent",
      "status": "completed",
      "start_time": "2024-01-13T10:00:00Z",
      "end_time": "2024-01-13T10:00:05Z"
    }],
    "spans": [
      {
        "trace_id": "my-agent-run-1",
        "span_id": "span-root",
        "name": "plan_research",
        "type": "agent",
        "status": "completed",
        "start_time": "2024-01-13T10:00:00Z",
        "end_time": "2024-01-13T10:00:01Z"
      },
      {
        "trace_id": "my-agent-run-1",
        "span_id": "span-llm-1",
        "parent_span_id": "span-root",
        "name": "gpt4_call",
        "type": "llm",
        "status": "completed",
        "start_time": "2024-01-13T10:00:01Z",
        "end_time": "2024-01-13T10:00:03Z",
        "model": "gpt-4",
        "total_tokens": 500,
        "total_cost": 0.015
      },
      {
        "trace_id": "my-agent-run-1",
        "span_id": "span-tool-1",
        "parent_span_id": "span-root",
        "name": "web_search",
        "type": "tool",
        "status": "completed",
        "start_time": "2024-01-13T10:00:03Z",
        "end_time": "2024-01-13T10:00:05Z",
        "input": {"query": "AI agents"},
        "output": {"results": ["result1", "result2"]}
      }
    ]
  }'
```

Response:
```json
{"status":"ok","batch_key":"demo-batch-001","trace_count":1,"span_count":3,"event_count":0,"accepted_count":4,"rejected_count":0}
```

#### Function Call Order

1. `server.Ingest()` [server.go:55]
2. `middleware.GetProjectID(ctx)` → `00000000-...`
3. `ingestService.Ingest(ctx, projectID, req)` [service.go:45]
4. `validateBatch(req)` [service.go:194]
5. `store.BeginTx()` [service.go:57]
6. `tx.ClaimBatch(ctx, projectID, "demo-batch-001")` [service.go:64]
7. `upsertTrace(ctx, tx, projectID, trace)` [service.go:83]
   - Returns internal UUID (e.g., `abc123-...`)
8. `upsertSpan(ctx, tx, projectID, traceUUID, span)` x3 [service.go:110]
9. `store.ComputeAndUpdateTraceRollupsTx(ctx, tx, traceUUID)` [service.go:146]
   - Query: `SELECT COUNT(*), SUM(total_tokens), SUM(total_cost), COUNT(*) FILTER WHERE status='failed'`
   - Update: `UPDATE traces SET total_spans=3, total_tokens=500, total_cost=0.015, error_count=0`
10. `tx.UpdateBatchStatus()` [service.go:162]
11. `tx.Commit()` [service.go:176]

#### Database Rows Created

**traces**:
```
id                                   | project_id      | trace_id        | name           | total_spans | total_tokens | total_cost | error_count
-------------------------------------+-----------------+-----------------+----------------+-------------+--------------+------------+-------------
abc123-...                           | 00000000-...    | my-agent-run-1  | research_agent | 3           | 500          | 0.015      | 0
```

**spans**:
```
id        | trace_id   | span_id     | parent_span_id | name          | type  | total_tokens | total_cost
----------+------------+-------------+----------------+---------------+-------+--------------+------------
def456-..| abc123-... | span-root   | NULL           | plan_research | agent | NULL         | NULL
ghi789-..| abc123-... | span-llm-1  | span-root      | gpt4_call     | llm   | 500          | 0.015
jkl012-..| abc123-... | span-tool-1 | span-root      | web_search    | tool  | NULL         | NULL
```

#### Step 4: Query via API

```bash
# List traces
curl -s -H "X-API-Key: test-api-key-12345" "http://localhost:8081/api/traces" | jq .

# Get spans (use internal trace UUID from list response)
curl -s -H "X-API-Key: test-api-key-12345" \
  "http://localhost:8081/api/traces/<trace-uuid>/spans" | jq .
```

#### Step 5: View in UI

```bash
# Start web dev server
make dev-web
# Open http://localhost:3000/traces
# Enter API key: test-api-key-12345
```

#### Error Cases

**Missing API key**:
```bash
curl -s http://localhost:8081/api/traces
# {"code":"missing_api_key","message":"API key required"}
# Status: 401
```

**Invalid API key**:
```bash
curl -s -H "X-API-Key: wrong-key" http://localhost:8081/api/traces
# {"code":"invalid_api_key","message":"Invalid API key"}
# Status: 401
```

**Duplicate batch key**:
```bash
# Same batch_key as before
curl -X POST ... -d '{"batch_key": "demo-batch-001", ...}'
# {"status":"duplicate","batch_key":"demo-batch-001"}
# Status: 200 (not an error - idempotent success)
```

---

## 9. Testing Guide

### Test Commands

| Command | Description |
|---------|-------------|
| `make test` | Run all tests (Go + JS) |
| `make test-go` | Run Go tests with race detector |
| `make test-js` | Run JavaScript/TypeScript tests |
| `make test-integration` | Run integration tests (requires DB) |
| `make ci` | Full CI pipeline (generate + lint + test) |

### Running Tests

```bash
# Go tests
make test-go
# Output: All PASS for pkg/truncation, etc.

# Web tests
cd web && pnpm test

# Python SDK tests
cd sdks/python && uv run pytest tests/
```

### Docker Compose Test Stack

```yaml
# deploy/docker-compose/docker-compose.dev.yml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: continua
      POSTGRES_PASSWORD: continua
      POSTGRES_DB: continua
    ports:
      - "5432:5432"
```

### Existing Tests (Phase 2 Relevant)

| Test File | Coverage |
|-----------|----------|
| `pkg/truncation/*_test.go` | Payload truncation logic |
| `sdks/python/tests/test_batch.py` | Batch queue accumulation |
| `sdks/python/tests/test_span.py` | Span context manager |
| `sdks/python/tests/test_trace.py` | Trace decorator |

### Test Gaps Identified

- **Auth middleware tests**: Missing unit tests for `APIKeyAuth`
- **Rollup computation tests**: No tests for `ComputeAndUpdateTraceRollups`
- **Multi-tenant isolation tests**: No tests verifying project scoping
- **Integration tests**: No E2E tests for full ingest-to-read flow
- **Web UI tests**: No component tests for TracesPage, SpanTree

---

## 10. Troubleshooting & Common Pitfalls

### Server Doesn't Start

**Symptom**: `DATABASE_URL environment variable is required`

**Fix**:
```bash
export DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable"
```

**Symptom**: `dial tcp [::1]:5432: connect: connection refused`

**Fix**:
```bash
# Start PostgreSQL
make dev
# Wait for it to be ready
docker ps  # Should show postgres container
```

### Auth Blocks Everything

**Symptom**: All requests return 401

**Check**: Is the API key hash correct in DB?
```bash
# Get stored hash
psql "postgres://continua:continua@localhost:5432/continua" \
  -c "SELECT api_key_hash FROM projects WHERE id = '00000000-0000-0000-0000-000000000001';"

# Compute expected hash
echo -n "test-api-key-12345" | shasum -a 256

# If they don't match, run migration
migrate -path db/platform/migrations/postgres -database "..." up
```

### UI Can't Talk to API

**Symptom**: `Network Error` or `CORS error`

**Check**: Is Vite proxying correctly?
```typescript
// web/vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8081',
      '/v1': 'http://localhost:8081'
    }
  }
});
```

**Fix**: Ensure PORT matches between backend and Vite config.

### Rollups Incorrect or Missing

**Symptom**: `total_tokens`, `total_cost` are NULL or 0

**Check**: Did the spans have token/cost data?
```bash
psql "..." -c "SELECT span_id, total_tokens, total_cost FROM spans WHERE trace_id = '...';"
```

**Check**: Were rollups computed?
```bash
# Look for warning logs during ingest
# "Warning: failed to compute rollups for trace..."
```

### Span Tree Doesn't Build

**Symptom**: All spans appear as root nodes in UI

**Check**: Are `span_id` and `parent_span_id` using external IDs?
```bash
curl -s -H "X-API-Key: ..." "http://localhost:8081/api/traces/{id}/spans" | jq '.spans[] | {span_id, parent_span_id}'
```

**Fix**: Ensure `span_id` field is included in API response (fixed in this phase).

### Debug Playbook

**1. Check server health**:
```bash
curl http://localhost:8081/api/health
```

**2. Check auth**:
```bash
curl -v -H "X-API-Key: test-api-key-12345" http://localhost:8081/api/traces
```

**3. Check DB connection**:
```bash
psql "postgres://continua:continua@localhost:5432/continua" -c "SELECT 1;"
```

**4. Check traces exist**:
```bash
psql "..." -c "SELECT id, name, total_spans FROM traces LIMIT 5;"
```

**5. Check logs**:
Server logs go to stdout. Look for:
- `Starting Continua server on 0.0.0.0:8081`
- `Warning: failed to compute rollups...`
- Any panic stack traces

---

## Appendix: Changed Files Summary

### Modified (M)
- `.air.toml` - Hot reload config
- `cmd/continua/main.go` - Fx wiring
- `contracts/openapi/openapi.yaml` - Schema extensions
- `contracts/generated/*` - Regenerated
- `internal/api/mapper.go` - New field mappings
- `internal/api/server.go` - Project scoping, session filter
- `internal/api/server_gen.go` - Regenerated
- `internal/ingest/service.go` - Rollup integration
- `sdks/python/src/continua/__init__.py` - Exports
- `sdks/python/src/continua/types.py` - Regenerated
- `web/src/App.tsx` - Routes
- `web/vite.config.ts` - Proxy config

### New (??)
- `internal/config/*` - Config loading, Fx module
- `internal/store/pool.go` - Pool creation
- `internal/store/module.go` - Fx module
- `internal/store/rollups.go` - Rollup methods
- `internal/ingest/module.go` - Fx module
- `internal/api/module.go` - Fx module
- `internal/api/router.go` - Router assembly
- `internal/api/middleware/auth.go` - Auth middleware
- `db/platform/queries/rollups.sql` - Aggregation query
- `db/platform/migrations/postgres/000002_*` - API key fix
- `sdks/python/src/continua/client.py` - Main client
- `sdks/python/src/continua/batch.py` - Batching
- `sdks/python/src/continua/trace.py` - Trace context
- `sdks/python/src/continua/span.py` - Span context
- `sdks/python/tests/*` - SDK tests
- `sdks/python/examples/e2e_demo.py` - Demo
- `web/src/api/client.ts` - API client
- `web/src/pages/TracesPage.tsx` - Traces list
- `web/src/pages/TraceDetailPage.tsx` - Trace detail
- `web/src/components/*` - UI components
- `web/src/utils/format.ts` - Formatters
