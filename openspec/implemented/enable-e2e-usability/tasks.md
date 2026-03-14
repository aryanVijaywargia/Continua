# Tasks: Enable End-to-End Platform Usability

## Spec 0: OpenAPI Schema Updates (Pre-requisite)

### 0.1 Remove Health from OpenAPI
- [ ] Edit `contracts/openapi/openapi.yaml`
  - Remove `/api/health` path definition (lines 12-23)
  - Health endpoint will be routed directly in Go code (public, no auth)
  - This enables clean auth middleware application to all OpenAPI routes

### 0.2 Extend Trace Schema
- [ ] Add `error_count` field to Trace schema
  ```yaml
  error_count:
    type: integer
    nullable: true
    description: Count of failed spans in this trace
  ```

### 0.3 Extend Span Schema
- [ ] Add `input` field to Span schema
  ```yaml
  input:
    type: object
    nullable: true
    description: Span input payload (JSON)
  ```
- [ ] Add `output` field to Span schema
  ```yaml
  output:
    type: object
    nullable: true
    description: Span output payload (JSON)
  ```
- [ ] **Change `parent_span_id` type from UUID to string**
  - Current schema (line 249-252) has `format: uuid` - REMOVE this
  - Change to plain string to match external ID format stored in DB
  - This allows direct mapping without UUID lookup
  ```yaml
  parent_span_id:
    type: string
    nullable: true
    description: External parent span identifier
  ```

### 0.4 Regenerate Code
- [ ] Run `make generate` to regenerate Go types and handlers
- [ ] Verify no drift: `git diff contracts/generated/`

### 0.5 Update Mapper
- [ ] Update `internal/api/mapper.go` - `traceToAPI()`
  - Add mapping for `error_count` from DB field
- [ ] Update `internal/api/mapper.go` - `spanToAPI()`
  - Add mapping for `parent_span_id` - **direct string copy from DB** (no UUID lookup needed after schema change)
  - Add mapping for `input` (parse JSON from DB bytes)
  - Add mapping for `output` (parse JSON from DB bytes)

**Exit Criteria**: OpenAPI schema extended, code regenerated, mappers updated.

---

## Spec 1: Server Bootstrap

### 1.1 Configuration (Env-Only for Phase 2)
- [ ] Create `internal/config/config.go`
  - Define `Config` struct with `Server`, `Database` fields
  - Implement `Load()` using `os.Getenv` with defaults
  - Server: `HOST` (0.0.0.0), `PORT` (8080)
  - Database: `DATABASE_URL` (required)
  - Note: `config.example.yaml` is for future phases; Phase 2 uses env vars only

### 1.2 Database Pool
- [ ] Create `internal/store/pool.go`
  - Implement `NewPool(cfg *config.Config) (*pgxpool.Pool, error)`
  - Parse connection string, set pool size defaults
  - Return configured pool

### 1.3 Fx Modules
- [ ] Create `internal/config/module.go`
  - Export `var Module = fx.Provide(Load)`
- [ ] Create `internal/store/module.go`
  - Export `var Module = fx.Module("store", fx.Provide(NewPool), fx.Provide(New))`
- [ ] Create `internal/ingest/module.go`
  - Export `var Module = fx.Provide(NewService)`
- [ ] Create `internal/api/module.go`
  - Export `var Module = fx.Module("api", fx.Provide(NewServer), fx.Provide(NewRouter))`

### 1.4 Router Assembly (Composition Pattern)
- [ ] Create `internal/api/router.go`
  - Implement `NewRouter(server *Server, store *store.Store, webHandler http.Handler) http.Handler`
  - Create Chi router with middleware: RequestID, Logger, Recoverer
  - Mount public health handler directly: `r.Get("/api/health", server.HealthCheck)`
  - Create protected group with auth middleware for OpenAPI routes:
    ```go
    r.Group(func(r chi.Router) {
        r.Use(middleware.APIKeyAuth(store))
        // Mount remaining OpenAPI handlers here
        HandlerWithOptions(server, ChiServerOptions{BaseRouter: r})
    })
    ```
  - Mount web UI handler at `/`

### 1.5 HTTP Server Lifecycle
- [ ] Update `cmd/continua/serve.go`
  - Replace TODO stub with `fx.New()`
  - Include all modules
  - Add `fx.Invoke(startServer)` for HTTP listen
  - Add lifecycle hooks for graceful shutdown

### 1.6 Verification
- [ ] Run `make generate` - no drift
- [ ] Run `make lint` - no errors
- [ ] Start server with `DATABASE_URL=... go run ./cmd/continua serve`
- [ ] Verify `GET /api/health` returns 200
- [ ] Verify `GET /api/traces` returns 200 (empty list)
- [ ] Write `docs/phase2/spec-1-server-bootstrap.md`

**Exit Criteria**: Server starts, endpoints respond, graceful shutdown works.

---

## Spec 2: Auth Enforcement

### 2.1 Auth Middleware
- [ ] Create `internal/api/middleware/auth.go`
  - Define `contextKey` type and `ProjectIDKey` constant
  - Implement `APIKeyAuth(store *store.Store) func(http.Handler) http.Handler`
  - Extract API key from `X-API-Key` header or `Authorization: Bearer`
  - Validate with `store.GetProjectByAPIKey()`
  - Inject project ID into context
  - Return 401 JSON for missing/invalid key

### 2.2 Context Helper
- [ ] Add `GetProjectID(ctx context.Context) (uuid.UUID, bool)` helper
  - Extract project ID from context for handlers

### 2.3 Router Update
- [ ] Update `internal/api/router.go`
  - Health endpoint routed directly (public, no auth) - done in Spec 1.4
  - All OpenAPI routes get auth middleware via group - done in Spec 1.4
  - No path-based bypass needed in middleware (clean separation)

### 2.4 Handler Updates (All Data Endpoints)
- [ ] Update `internal/api/server.go` handlers to use project context
  - `Ingest()`: Get project ID from context, pass to ingest service
  - `ListTraces()`: Filter by project ID (add WHERE project_id = $X)
  - `GetTrace()`: Verify trace belongs to project ID (return 404 if not)
  - `ListSpansByTrace()`: Verify parent trace belongs to project ID
  - `ListSessions()`: Filter by project ID

### 2.5 Test Project Seeding
- [ ] Create seed SQL for test project
  - Project with known API key for testing
  - Add to `db/platform/migrations/` or test helper

### 2.6 Verification
- [ ] Run `make test-go`
- [ ] Test: `curl /api/health` → 200
- [ ] Test: `curl /api/traces` → 401
- [ ] Test: `curl -H "X-API-Key: invalid" /api/traces` → 401
- [ ] Test: `curl -H "X-API-Key: <valid>" /api/traces` → 200
- [ ] Write `docs/phase2/spec-2-auth.md`

**Exit Criteria**: All data endpoints require valid API key, health is public.

---

## Spec 3: Python SDK

### 3.1 Client Implementation
- [ ] Create `sdks/python/src/continua/client.py`
  - `Continua` class with `api_key`, `endpoint`, `httpx.Client`
  - `_instance` class variable for singleton access
  - `ingest(batch)` method for synchronous POST
  - `flush()` and `shutdown()` methods
  - `get_instance()` class method

### 3.2 Batch Queue
- [ ] Create `sdks/python/src/continua/batch.py`
  - `BatchQueue` class with threading.Lock
  - `add_trace()`, `add_span()`, `add_event()` methods
  - Background flush thread with configurable interval
  - `flush()` for immediate send, `shutdown()` for cleanup

### 3.3 Trace Context
- [ ] Create `sdks/python/src/continua/trace.py`
  - `_current_trace` contextvars.ContextVar
  - `TraceContext` class with `__enter__`/`__exit__`
  - `set_input()`, `set_output()` methods
  - `@trace` decorator for function tracing
  - `get_current_trace()` helper

### 3.4 Span Context
- [ ] Create `sdks/python/src/continua/span.py`
  - `_current_span` contextvars.ContextVar
  - `SpanContext` class with `__enter__`/`__exit__`
  - `set_input()`, `set_output()`, `set_model()`, `set_tokens()` methods
  - `span()` function returning SpanContext
  - Parent span ID propagation

### 3.5 Public API
- [ ] Update `sdks/python/src/continua/__init__.py`
  - Export: `Continua`, `trace`, `span`, `TraceContext`, `SpanContext`
  - Keep generated types importable

### 3.6 Unit Tests
- [ ] Create `sdks/python/tests/test_trace.py`
  - Test decorator creates trace
  - Test nested spans link correctly
- [ ] Create `sdks/python/tests/test_batch.py`
  - Test batch queue accumulates items
  - Test flush sends to callback

### 3.7 Integration Test
- [ ] Create `sdks/python/tests/test_integration.py`
  - Test against running server
  - Verify data appears in API

### 3.8 Verification
- [ ] Run `cd sdks/python && pip install -e .`
- [ ] Run `python -m pytest tests/`
- [ ] Write `docs/phase2/spec-3-python-sdk.md`

**Exit Criteria**: SDK installs, decorators work, data reaches server.

---

## Spec 4: Trace Rollups

### 4.1 Aggregation Query
- [ ] Create `db/platform/queries/rollups.sql`
  - Add `ComputeTraceRollups` query
  - Aggregate: COUNT spans, SUM tokens, SUM cost, COUNT errors

### 4.2 Store Method
- [ ] Create `internal/store/rollups.go`
  - Define `TraceRollups` struct
  - Implement `ComputeTraceRollupsTx(ctx, tx, traceID) (*TraceRollups, error)`

### 4.3 Ingest Integration
- [ ] Update `internal/ingest/service.go`
  - After upserting spans, collect affected trace IDs
  - For each trace, compute and update rollups
  - Log warning on rollup error, don't abort transaction

### 4.4 Verification
- [ ] Run `make generate`
- [ ] Run `make test-go`
- [ ] Ingest trace with spans via API
- [ ] Query trace, verify `total_tokens`, `total_cost`, `total_spans` populated
- [ ] Write `docs/phase2/spec-4-rollups.md`

**Exit Criteria**: After ingest, traces have computed rollup values.

---

## Spec 5: Web UI - Traces List

### 5.1 API Client
- [ ] Create `web/src/api/client.ts`
  - `fetchAPI<T>(path, options)` with API key header
  - Read key from `localStorage.getItem('continua_api_key')`
  - Error handling for 401, 404, 500

### 5.2 API Key Prompt
- [ ] Create `web/src/components/ApiKeyPrompt.tsx`
  - Input field for API key
  - Save to localStorage on submit
  - Minimal styling with Tailwind

### 5.3 Traces Page (with limit/offset pagination)
- [ ] Create `web/src/pages/TracesPage.tsx`
  - Check for API key, show prompt if missing
  - Use `useQuery` from TanStack Query
  - Fetch `/api/traces?limit=${PAGE_SIZE}&offset=${offset}` (use existing API params)
  - **Use limit/offset pagination** (NOT page param - API uses limit/offset):
    - PAGE_SIZE = 20
    - State: `offset` (starts at 0)
    - Next: `offset += PAGE_SIZE`
    - Previous: `offset = max(0, offset - PAGE_SIZE)`
  - Display table: name, status, duration, tokens, cost, timestamp
  - Status filter dropdown (add as query param if API supports)
  - Pagination controls (Previous/Next using offset)
  - Link rows to `/traces/:id`

### 5.4 Status Badge Component
- [ ] Create `web/src/components/StatusBadge.tsx`
  - Color-coded badge for running/completed/failed

### 5.5 Format Utilities
- [ ] Create `web/src/utils/format.ts`
  - `formatDuration(ms)` → human readable
  - `formatTokens(count)` → with K/M suffix
  - `formatCost(amount)` → currency format
  - `formatRelativeTime(date)` → "2m ago"

### 5.6 Verification
- [ ] Run `cd web && pnpm dev`
- [ ] Open http://localhost:5173/traces
- [ ] Enter API key when prompted
- [ ] Verify traces table renders
- [ ] Verify pagination works
- [ ] Verify status filter works
- [ ] Write `docs/phase2/spec-5-ui-traces-list.md`

**Exit Criteria**: Traces page shows data with filtering and pagination.

---

## Spec 6: Web UI - Trace Detail

### 6.1 Trace Detail Page
- [ ] Create `web/src/pages/TraceDetailPage.tsx`
  - Use `useParams` for trace ID
  - Fetch trace and spans with `useQuery`
  - Header: name, status, duration, tokens, cost, error count
  - Split layout: span tree left, detail panel right

### 6.2 Span Tree Component
- [ ] Create `web/src/components/SpanTree.tsx`
  - Build tree structure from flat spans array
  - Recursive `SpanNode` component
  - Expand/collapse children
  - Selection highlight
  - Duration and status indicators

### 6.3 Span Detail Component
- [ ] Create `web/src/components/SpanDetail.tsx`
  - Show span name, type, status, model, tokens
  - JSON display for input/output
  - Scrollable with max height

### 6.4 Type Icons
- [ ] Create `web/src/components/TypeIcon.tsx`
  - Different icons for llm, tool, retrieval, agent, custom
  - Use lucide-react icons

### 6.5 Status Dot
- [ ] Create `web/src/components/StatusDot.tsx`
  - Small colored dot for span status

### 6.6 Verification
- [ ] Navigate to `/traces/:id`
- [ ] Verify header shows trace metadata
- [ ] Verify span tree renders with hierarchy
- [ ] Verify expand/collapse works
- [ ] Verify clicking span shows detail
- [ ] Write `docs/phase2/spec-6-ui-trace-detail.md`

**Exit Criteria**: Trace detail page shows span tree with interactive detail panel.

---

## Dependencies

```
Spec 1 (Server Bootstrap)
    ↓
Spec 2 (Auth) ─────→ Spec 3 (Python SDK) [can test against server]
    ↓
Spec 4 (Rollups)
    ↓
Spec 5 (UI Traces List)
    ↓
Spec 6 (UI Trace Detail)
```

**Parallel Tracks**:
- Track A (Backend): Specs 1 → 2 → 4
- Track B (SDK): Spec 3 (after Spec 2)
- Track C (UI): Specs 5 → 6 (after Spec 1, uses any auth approach)

---

## Notes

### Anti-Drift Checklist (Before Each Spec)
- [ ] Read existing code first (discovery complete)
- [ ] Schema columns exist (verified in Spec 0)
- [ ] Endpoints exist in OpenAPI (verified in Spec 0)
- [ ] Dependencies already installed (verified in Spec 0)
- [ ] Know test command (`make test-go`, `pnpm test`)

### Output Format (After Each Spec)
1. Run tests
2. Write `docs/phase2/spec-X-<name>.md` summary
3. Print `=== SPEC X COMPLETE: READY FOR REVIEW ===`
4. STOP and wait for approval

### Constraints
- No new Go dependencies
- Python SDK: only httpx (already approved)
- Web UI: use existing packages only
- Stop after each spec for approval
