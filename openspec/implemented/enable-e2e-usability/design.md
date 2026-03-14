# Design: Enable End-to-End Platform Usability

## Context

Continua has a complete data layer but no working server. This design documents the architectural decisions for wiring components together with minimal new code.

### Stakeholders
- Backend: Go developers extending API
- SDK users: Python developers instrumenting agents
- Frontend: React developers building UI
- Operators: DevOps running the platform

### Constraints
1. No new Go dependencies (Fx already in go.mod, just unused)
2. Python SDK: only httpx approved (pydantic already present)
3. Web UI: use existing packages (TanStack Query, react-router-dom)
4. Stop after each spec, run tests, wait for approval

---

## Goals / Non-Goals

### Goals
- Make `continua serve` start a working HTTP server
- Enforce API key authentication on all data endpoints
- Provide Python SDK for tracing AI agents
- Compute and store trace rollups
- Display traces and spans in web UI

### Non-Goals
- Async rollup processing (defer to v1.1 with River)
- WebSocket real-time updates (defer to v1.1)
- User/team management (v2 scope)
- TypeScript SDK (Python first)

---

## Decisions

### Decision 1: Minimal Fx Module Structure

**Choice**: Create 4 Fx modules, wire in serve command

**Rationale**:
- Fx is already in go.mod but unused
- All constructors are ready (NewStore, NewServer)
- Only need to provide pool creation

**Structure**:
```
cmd/continua/serve.go     → fx.New() with modules
internal/config/module.go → fx.Provide(Load)
internal/store/module.go  → fx.Provide(NewPool, New)
internal/api/module.go    → fx.Provide(NewRouter, NewServer)
internal/ingest/module.go → fx.Provide(NewService)
```

**Alternatives Considered**:
- Manual wiring without Fx → More boilerplate, no lifecycle management
- Full Fx module per package → Over-engineering for current needs

---

### Decision 2: Router Composition Pattern

**Choice**: Health endpoint routed directly (public), OpenAPI handlers mounted under auth middleware group

**Rationale**:
- `/api/health` removed from OpenAPI spec to avoid middleware conflict
- Health handler registered directly in Chi router (public, no auth)
- All OpenAPI-defined routes receive auth middleware via group
- No path-based bypass logic needed in middleware (clean separation)

**Flow**:
```go
// internal/api/router.go
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

**Alternatives Considered**:
- Path-based bypass in middleware → More complex, error-prone
- Separate Chi instances → Unnecessary complexity

---

### Decision 3: Python SDK Architecture

**Choice**: Singleton client with context vars and background flush

**Rationale**:
- Context vars provide async-safe trace/span propagation
- Background thread enables non-blocking batching
- Singleton pattern simplifies decorator usage

**Structure**:
```python
client.py  → Continua class with httpx.Client, BatchQueue
trace.py   → TraceContext using contextvars, @trace decorator
span.py    → SpanContext using contextvars, span() context manager
batch.py   → BatchQueue with threading.Lock, background flush
types.py   → Already generated from OpenAPI (keep as-is)
```

**Alternatives Considered**:
- Async-only (asyncio) → Limits sync framework usage
- No batching → Too many HTTP requests
- Per-request clients → Memory inefficient

---

### Decision 4: Inline Rollup Computation

**Choice**: Compute rollups at end of ingest transaction

**Rationale**:
- Schema already has rollup columns
- `UpdateTraceRollups` SQLC query exists
- Inline is simpler for v1 (no job queue)
- Can defer async to v1.1 with River

**Implementation**:
```go
// internal/ingest/service.go - after upserting spans
for _, traceUUID := range affectedTraces {
    rollups := computeRollups(ctx, tx, traceUUID) // aggregate query
    store.UpdateTraceRollups(ctx, tx, traceUUID, rollups)
}
```

**Alternatives Considered**:
- Post-commit job → Requires River setup (defer to v1.1)
- Trigger-based → Harder to debug, less control

---

### Decision 5: Web UI Data Fetching

**Choice**: Native fetch with API key header, wrapped in TanStack Query, using limit/offset pagination

**Rationale**:
- TanStack Query already installed
- No need for Axios (native fetch sufficient)
- API key from localStorage for simplicity
- **Use existing API pagination**: `limit` (default 50) and `offset` parameters (NOT `page`)

**Pattern**:
```typescript
// api/client.ts
export async function fetchAPI<T>(path: string): Promise<T> {
  const apiKey = localStorage.getItem('continua_api_key');
  const res = await fetch(path, {
    headers: { 'X-API-Key': apiKey || '' }
  });
  if (!res.ok) throw new Error(res.statusText);
  return res.json();
}

// pages/TracesPage.tsx - using limit/offset (matches OpenAPI)
const PAGE_SIZE = 20;
const [offset, setOffset] = useState(0);

const { data } = useQuery({
  queryKey: ['traces', offset],
  queryFn: () => fetchAPI<TracesResponse>(`/api/traces?limit=${PAGE_SIZE}&offset=${offset}`)
});

// Pagination: next page = offset + PAGE_SIZE
const nextPage = () => setOffset(o => o + PAGE_SIZE);
const prevPage = () => setOffset(o => Math.max(0, o - PAGE_SIZE));
```

**Alternatives Considered**:
- Add Axios → Unnecessary, fetch is sufficient
- Add `page` parameter to API → Unnecessary, limit/offset already exists
- Server-side auth → Not applicable for SPA

---

## Risks / Trade-offs

### Risk: Inline Rollups Add Latency
- **Impact**: Ingest requests take longer
- **Mitigation**: Aggregation query is simple SUM, not a concern for v1 scale
- **Future**: Move to async with River in v1.1

### Risk: Python SDK Thread Safety
- **Impact**: Concurrent traces could corrupt state
- **Mitigation**: Context vars for isolation, Lock for batch queue
- **Testing**: Add concurrent test case

### Risk: No API Key Rotation
- **Impact**: Compromised keys require DB update
- **Mitigation**: v1 is internal/dev use only
- **Future**: Add key rotation API in v2

---

## Migration Plan

Not applicable - this enables new functionality on a non-production system.

---

## Open Questions

1. **Q**: Should health endpoint require auth?
   **A**: No - keep public for load balancer probes

2. **Q**: Should ingest rollup failures abort the transaction?
   **A**: No - log warning, continue (rollups are eventually consistent)

3. **Q**: API key format?
   **A**: `ck_live_<32 hex chars>` (matches existing convention in specs)

---

## References

- Discovery findings: [docs/phase2/spec-0-discovery.md](../../../docs/phase2/spec-0-discovery.md)
- Existing ingestion design: [../add-ingestion-pipeline/design.md](../add-ingestion-pipeline/design.md)
