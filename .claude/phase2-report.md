# Continua Phase 2: End-to-End Platform Usability Report

**Date**: January 2026
**Branch**: `phase2-e2e-usability`
**PR**: #22

---

## Executive Summary

Phase 2 transformed Continua from a collection of unconnected components into a fully functional AI Agent Observability Platform. Before Phase 2, the server wouldn't start (Fx DI incomplete), authentication didn't exist, there was no SDK to send data, and the web UI was empty placeholders. Phase 2 wired everything together with minimal new code, making Continua actually usable end-to-end.

---

## What Was Built

### 1. Server Bootstrap (Spec 1)

**Problem**: `continua serve` was a TODO stub. The server didn't start.

**Solution**: Implemented Uber Fx dependency injection with 4 modules:

| Module | Location | Purpose |
|--------|----------|---------|
| Config | `internal/config/module.go` | Environment variable loading |
| Store | `internal/store/module.go` | PostgreSQL pool + store initialization |
| API | `internal/api/module.go` | Router + HTTP server |
| Ingest | `internal/ingest/module.go` | Trace/span ingestion service |

**New Files**:
- `internal/config/config.go` - Config struct with `Server`, `Database` fields
- `internal/config/module.go` - Fx module export
- `internal/store/pool.go` - pgxpool.Pool creation
- `internal/store/module.go` - Fx module export
- `internal/api/router.go` - Chi router assembly
- `internal/api/module.go` - Fx module export
- `internal/ingest/module.go` - Fx module export

**Modified Files**:
- `cmd/continua/serve.go` - Replaced TODO with `fx.New()` app with lifecycle hooks

**Configuration** (env vars):
```bash
HOST=0.0.0.0        # Server host (default)
PORT=8080           # Server port (default)
DATABASE_URL=...    # PostgreSQL connection string (required)
```

---

### 2. Authentication Middleware (Spec 2)

**Problem**: All endpoints were public. No multi-tenancy support.

**Solution**: API key middleware with project context injection.

**New Files**:
- `internal/api/middleware/auth.go`

**How It Works**:
```go
// Extracts API key from either header
X-API-Key: ck_live_abc123...
// or
Authorization: Bearer ck_live_abc123...

// Validates against database
store.GetProjectByAPIKey(ctx, apiKey)

// Injects project ID into context for handlers
context.WithValue(ctx, ProjectIDKey, projectID)
```

**Router Architecture** (composition pattern):
```go
r := chi.NewRouter()
r.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)

// Public: health endpoint (NOT in OpenAPI)
r.Get("/api/health", server.HealthCheck)

// Protected: all OpenAPI routes
r.Group(func(r chi.Router) {
    r.Use(middleware.APIKeyAuth(store))
    HandlerWithOptions(server, ChiServerOptions{BaseRouter: r})
})
```

**Key Decision**: Removed `/api/health` from OpenAPI spec to avoid middleware bypass logic. Health is routed directly in Chi (public), while all OpenAPI routes get auth middleware via group.

---

### 3. Python SDK (Spec 3)

**Problem**: No way to send traces from Python applications.

**Solution**: Full-featured SDK with decorators, context managers, and automatic batching.

**New Files**:
| File | Purpose |
|------|---------|
| `sdks/python/src/continua/client.py` | HTTP client with httpx, singleton pattern |
| `sdks/python/src/continua/trace.py` | `@trace` decorator, `TraceContext` class |
| `sdks/python/src/continua/span.py` | `span()` context manager, `SpanContext` class |
| `sdks/python/src/continua/batch.py` | Background queue with threading.Lock |
| `sdks/python/src/continua/types.py` | Pydantic models (generated from OpenAPI) |
| `sdks/python/src/continua/__init__.py` | Public API exports |

**Usage Example**:
```python
from continua import Continua, trace, span

# Initialize client
client = Continua(api_key="ck_live_...", endpoint="http://localhost:8080")

@trace(name="my-agent")
def run_agent(query: str):
    with span(name="llm-call", kind="LLM") as s:
        s.set_model("gpt-4")
        response = call_openai(query)
        s.set_tokens(input=100, output=50)
        s.set_output(response)
    return response

# Traces are automatically batched and sent
run_agent("Hello world")
client.shutdown()  # Flush remaining traces
```

**Architecture**:
- **Context vars**: Async-safe trace/span propagation
- **Singleton client**: Simplifies decorator usage
- **Background thread**: Non-blocking batch flush (configurable interval)
- **Thread safety**: `threading.Lock` protects batch queue

---

### 4. Trace Rollups (Spec 4)

**Problem**: Trace aggregates (total tokens, cost, span count) weren't computed.

**Solution**: Inline rollup computation at end of ingest transaction.

**New/Modified Files**:
- `db/platform/queries/rollups.sql` - Aggregation query
- `internal/store/rollups.go` - `TraceRollups` struct and store method
- `internal/ingest/service.go` - Trigger rollups after span upsert

**Aggregated Fields**:
```sql
UPDATE traces SET
    total_tokens_in = SUM(spans.tokens_in),
    total_tokens_out = SUM(spans.tokens_out),
    total_cost_usd = SUM(spans.cost_usd),
    total_spans = COUNT(spans),
    error_count = COUNT(spans WHERE status = 'FAILED')
WHERE id = $trace_id;
```

**Design Decision**: Inline computation (not async) for v1 simplicity. Async processing with River deferred to v1.1.

---

### 5. Web UI - Traces List (Spec 5)

**Problem**: Traces page was an empty placeholder.

**Solution**: Full traces table with filtering and pagination.

**New Files**:
| File | Purpose |
|------|---------|
| `web/src/pages/TracesPage.tsx` | Main traces list page |
| `web/src/components/ApiKeyPrompt.tsx` | API key input modal |
| `web/src/components/StatusBadge.tsx` | Color-coded status badges |
| `web/src/api/client.ts` | Fetch wrapper with API key header |
| `web/src/utils/format.ts` | Duration, tokens, cost formatters |

**Features**:
- API key stored in localStorage
- Table columns: name, status, duration, tokens, cost, timestamp
- Status filter dropdown (RUNNING, COMPLETED, FAILED)
- Limit/offset pagination (PAGE_SIZE = 20)
- Links to trace detail page

**Data Fetching Pattern**:
```typescript
const { data } = useQuery({
  queryKey: ['traces', offset, statusFilter],
  queryFn: () => fetchAPI<TracesResponse>(
    `/api/traces?limit=${PAGE_SIZE}&offset=${offset}&status=${statusFilter}`
  )
});
```

---

### 6. Web UI - Trace Detail (Spec 6)

**Problem**: No way to visualize trace execution.

**Solution**: Interactive span tree with detail panel.

**New Files**:
| File | Purpose |
|------|---------|
| `web/src/pages/TraceDetailPage.tsx` | Trace detail with split layout |
| `web/src/components/SpanTree.tsx` | Recursive tree with expand/collapse |
| `web/src/components/SpanDetail.tsx` | JSON viewer for input/output |

**Features**:
- Header: trace name, status, duration, tokens, cost, error count
- Left panel: hierarchical span tree
- Right panel: selected span details
- Expand/collapse subtrees
- Type icons (LLM, TOOL, AGENT, CHAIN, CUSTOM)
- Duration and status indicators per span

**Tree Building**:
```typescript
// Spans have parent_span_id for hierarchy
interface SpanNode {
  span: Span;
  children: SpanNode[];
}

function buildTree(spans: Span[]): SpanNode[] {
  // Group by parent_span_id, recursively build tree
}
```

---

### 7. OpenAPI Schema Updates (Spec 0)

**Changes to `contracts/openapi/openapi.yaml`**:

| Change | Before | After |
|--------|--------|-------|
| `/api/health` | In OpenAPI | Removed (routed directly) |
| `Trace.error_count` | Missing | Added (integer, nullable) |
| `Span.input` | Missing | Added (object, nullable) |
| `Span.output` | Missing | Added (object, nullable) |
| `Span.parent_span_id` | `format: uuid` | Plain string |

**Mapper Updates** (`internal/api/mapper.go`):
- `traceToAPI()`: Maps `error_count` from DB
- `spanToAPI()`: Maps `parent_span_id`, `input`, `output`

---

## CI/CD Improvements

### E2E Test Workflow

**New File**: `.github/workflows/e2e.yml`

**What It Does**:
1. Spins up PostgreSQL 16 Alpine via GitHub Actions services
2. Builds Go server binary
3. Runs database migrations
4. Starts server with health check wait (30s timeout)
5. Installs Python SDK via uv
6. Runs `e2e_demo.py` integration test
7. Collects logs on failure

**Local Testing**:
```bash
make e2e  # Runs same flow locally
```

### CodeRabbit Configuration

**Changed**: Removed test file exclusions from `.coderabbit.yaml`
- Previously excluded: `!**/*_test.go`, `!**/*.test.ts`, `!**/*.spec.ts`
- Now: All test files reviewed

### CI Failure Policy

**Added to `CLAUDE.md`**:
```markdown
## Test and CI Failure Policy
- Never bypass, skip, or remove failing tests
- Never disable CI checks to make builds pass
- Always perform root cause analysis (RCA) first
- Fix the actual issue, not the symptom
```

---

## Commits (Chronological)

| Commit | Type | Description |
|--------|------|-------------|
| `e22ed75` | feat | Add auth middleware, session pagination, rollups |
| `599d73a` | feat | Allow any JSON type for span input/output |
| `85868aa` | feat | Server bootstrap with Fx DI |
| `9e575a3` | feat | Python SDK with batching and decorators |
| `24eea22` | feat | Web UI traces list and trace detail pages |
| `21ec513` | docs | Phase 2 specs and architecture docs |
| `7aec5e4` | fix | TypeScript and Go linter errors |
| `c4d5414` | fix | OpenAPI 3.1 spec compliance |
| `d085c4a` | chore | Regenerate code with latest oapi-codegen |
| `99780b7` | chore | Regenerate sqlc with v1.30.0 |
| `ed48aec` | fix | CodeRabbit review feedback |
| `a46dac2` | fix | Second round CodeRabbit feedback |
| `87c0445` | chore | Enable test file reviews, add CI policy |
| `fa180b3` | feat | E2E CI workflow with GitHub Actions services |
| `dfbf8fd` | docs | Add rules from diary reflection |

---

## Architecture Patterns Established

### 1. Router Composition
Health endpoint public, OpenAPI routes protected via Chi group.

### 2. Fx Module Structure
One module per package, minimal wiring in serve command.

### 3. Context Propagation
Project ID in Go context, trace/span in Python contextvars.

### 4. Contract-First Development
OpenAPI is source of truth → generate Go/Python types → implement handlers.

### 5. Inline Aggregation
Compute rollups in same transaction as ingest (async deferred to v1.1).

---

## File Structure After Phase 2

```
cmd/continua/
  └── serve.go              # Fx app with lifecycle hooks

internal/
  ├── api/
  │   ├── middleware/
  │   │   └── auth.go       # API key validation
  │   ├── router.go         # Chi router assembly
  │   ├── module.go         # Fx module
  │   ├── mapper.go         # Domain → API mapping
  │   └── server.go         # HTTP handlers
  ├── config/
  │   ├── config.go         # Env var loading
  │   └── module.go         # Fx module
  ├── store/
  │   ├── pool.go           # pgxpool creation
  │   ├── module.go         # Fx module
  │   └── rollups.go        # Trace aggregation
  └── ingest/
      ├── service.go        # Ingest + rollup trigger
      └── module.go         # Fx module

sdks/python/src/continua/
  ├── __init__.py           # Public API
  ├── client.py             # HTTP client
  ├── trace.py              # @trace decorator
  ├── span.py               # span() context manager
  ├── batch.py              # Background queue
  └── types.py              # Pydantic models

web/src/
  ├── pages/
  │   ├── TracesPage.tsx    # Traces list
  │   └── TraceDetailPage.tsx
  ├── components/
  │   ├── ApiKeyPrompt.tsx
  │   ├── SpanTree.tsx
  │   ├── SpanDetail.tsx
  │   └── StatusBadge.tsx
  ├── api/
  │   └── client.ts         # Fetch wrapper
  └── utils/
      └── format.ts         # Formatters

.github/workflows/
  └── e2e.yml               # E2E test workflow
```

---

## Tech Stack Summary

| Layer | Technologies |
|-------|--------------|
| Backend | Go 1.22, Chi router, Uber Fx, pgx/v5, sqlc |
| Database | PostgreSQL 16 |
| Python SDK | httpx, pydantic, contextvars |
| Web UI | Vite, React 18, TypeScript 5.6, TanStack Query, Tailwind |
| CI/CD | GitHub Actions, CodeRabbit |

---

## What's NOT in Phase 2 (Deferred)

| Feature | Deferred To | Reason |
|---------|-------------|--------|
| Async rollups | v1.1 | Requires River job queue setup |
| WebSocket real-time | v1.1 | Adds complexity |
| TypeScript SDK | v1.1 | Python SDK first |
| User/team management | v2 | Multi-tenancy scope |
| API key rotation | v2 | v1 is internal/dev use |

---

## Success Criteria (All Met)

- [x] `continua serve` starts without errors
- [x] `GET /api/health` returns 200
- [x] `GET /api/traces` requires valid API key (401 without)
- [x] Python SDK can send traces and they appear in API
- [x] Web UI shows traces list with filtering
- [x] Web UI shows trace detail with span tree
- [x] Trace rollups computed after ingest
- [x] `make ci` passes
- [x] E2E workflow validates full stack

---

## Key Learnings & Conventions

1. **No Co-Authored-By** in commits
2. **Feature branches** for all changes (never commit to main)
3. **`make generate`** after any contract/schema change
4. **Never manually edit** `*_gen.go` or `db/gen/` files
5. **RCA-first** for test/CI failures
6. **OpenSpec** for architectural changes
7. **Conventional commits**: `type: description`
