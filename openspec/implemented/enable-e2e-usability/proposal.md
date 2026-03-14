# Change: Enable End-to-End Platform Usability

## Summary

Wire together existing components to make Continua runnable and usable end-to-end: working server, authentication, Python SDK, trace rollups, and web UI.

## Why

Continua has a complete data layer (store, ingest service, schema) but the platform isn't usable because:
1. Server doesn't start (Fx DI incomplete - serve command is a TODO stub)
2. Authentication isn't enforced (no middleware exists)
3. No SDK to send data (Python SDK has types but no client)
4. No UI to view data (pages are empty placeholders)
5. Rollups don't compute (query exists but not triggered)

This change makes Continua **actually work** end-to-end by wiring existing code.

## What Changes

### NEW Capabilities
- **server-bootstrap**: Fx modules for DI, config loading, HTTP server lifecycle
- **authentication**: API key middleware with multi-tenant project context
- **python-sdk**: HTTP client, trace/span decorators, automatic batching
- **web-ui**: Traces list page, trace detail page with span tree visualization

### MODIFIED Capabilities
- **trace-rollups**: Trigger rollup computation after ingest

### Key Implementation Notes
1. **Minimal new code**: Most handlers/store methods already exist
2. **No new Go dependencies**: Only wire existing (Fx already in go.mod)
3. **Python SDK deps**: httpx (approved), pydantic (already present)
4. **Web UI**: Use existing TanStack Query and react-router-dom

## Impact

### Affected Specs
| Spec | Type | Description |
|------|------|-------------|
| server-bootstrap | NEW | Fx modules, config, HTTP lifecycle |
| authentication | NEW | API key middleware, project context |
| python-sdk | NEW | Client, decorators, batching |
| trace-rollups | MODIFIED | Trigger computation in ingest |
| web-ui | NEW | Traces list, trace detail pages |

### Affected Code
| Path | Change |
|------|--------|
| `contracts/openapi/openapi.yaml` | MODIFY: Remove `/api/health`, add fields to Trace/Span schemas |
| `cmd/continua/serve.go` | REPLACE stub with Fx app |
| `internal/config/config.go` | NEW config loading (env-only for Phase 2) |
| `internal/api/middleware/auth.go` | NEW auth middleware |
| `internal/api/router.go` | NEW router assembly (health outside OpenAPI, protected routes inside) |
| `internal/api/mapper.go` | MODIFY: Map `parent_span_id`, `input`, `output`, `error_count` |
| `internal/api/server.go` | MODIFY: Add project scoping to GetTrace, ListSpansByTrace |
| `internal/ingest/service.go` | MODIFY to compute rollups |
| `sdks/python/src/continua/` | NEW client, trace, span, batch |
| `web/src/pages/` | NEW TracesPage, TraceDetailPage |
| `web/src/components/` | NEW SpanTree, SpanDetail |
| `web/src/api/` | NEW API client |

### Breaking Changes
None - this adds new functionality and wires existing code.

### Schema Changes (OpenAPI)
| Schema | Field | Change |
|--------|-------|--------|
| Trace | `error_count` | ADD: integer, nullable |
| Span | `input` | ADD: object, nullable (JSON payload) |
| Span | `output` | ADD: object, nullable (JSON payload) |
| Span | `parent_span_id` | EXISTING but mapper drops it - FIX mapper |

### Router Architecture Change
- Remove `/api/health` from OpenAPI spec
- Route health handler directly in Chi router (public, no middleware)
- Apply auth middleware only to OpenAPI-generated routes
- This avoids middleware bypass logic in auth handler

## Dependencies

### Go (already in go.mod)
- `go.uber.org/fx v1.23.0` - DI framework (unused, now wiring)
- `github.com/go-chi/chi/v5` - HTTP router (already used in generated code)
- `github.com/jackc/pgx/v5` - PostgreSQL driver (already used)

### Python SDK
- `httpx>=0.25.0` - HTTP client (approved, already in pyproject.toml)
- `pydantic>=2.0.0` - Types (already present)

### Web UI (already in package.json)
- `@tanstack/react-query` - Data fetching
- `react-router-dom` - Routing
- `tailwindcss` - Styling
- `lucide-react` - Icons

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Fx wiring complexity | Start minimal - only 4 modules needed |
| Auth breaking existing flows | Health endpoint remains public |
| Python SDK thread safety | Use context vars + threading.Lock |
| Rollup performance | Compute inline (v1), defer async to v1.1 |

## Success Criteria

- [ ] `continua serve` starts without errors
- [ ] `GET /api/health` returns 200
- [ ] `GET /api/traces` requires valid API key (401 without)
- [ ] Python SDK can send traces and they appear in API
- [ ] Web UI shows traces list with filtering
- [ ] Web UI shows trace detail with span tree
- [ ] Trace rollups computed after ingest
- [ ] `make ci` passes

## Related Documents

- Discovery: [docs/phase2/spec-0-discovery.md](../../../docs/phase2/spec-0-discovery.md)
- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
