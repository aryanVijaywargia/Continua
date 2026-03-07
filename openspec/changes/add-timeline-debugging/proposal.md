# Change: Add Timeline Debugging Experience (Phase 4)

## Why

Users can ingest and store traces reliably (Phase 3), but cannot quickly understand what happened during a trace execution. The trace detail page shows a span tree and span details, but lacks a unified chronological view that merges explicit events (logs, errors) with span lifecycle transitions. Debugging requires mentally reconstructing order from scattered span data.

Phase 4 makes Continua **useful for debugging** by adding a merged events timeline, live polling for active traces, Python SDK event helper improvements, and contract alignment.

## What Changes

### NEW Capabilities

| Capability | Description |
|------------|-------------|
| **events-timeline** | `GET /api/traces/{id}/events` endpoint returning merged explicit events + synthetic span lifecycle markers, with opaque cursor pagination |
| **timeline-ui** | Chronological timeline view in trace detail page with visual distinction between event types |
| **long-polling** | Incremental polling for active traces using timeline cursor, stops on terminal state |
| **python-sdk-events** | `span.error()`, `span.exception()`, `span.metric()` helpers alongside existing `span.log()` |

### MODIFIED Capabilities

| Capability | Description |
|------------|-------------|
| **contract-alignment** | Align OpenAPI, backend, Python SDK types, and web client types for timeline contracts |

### Key Design Decisions

1. **Timeline is a merged VIEW, not new storage** — explicit events from `span_events`, synthetic lifecycle markers computed from `spans` at query time
2. **Opaque cursor pagination** — monotonic cursor key for stable incremental polling; full refresh on terminal state for accurate ordering
3. **No trace-level events** — current schema requires `span_id` on events; no schema change in Phase 4
4. **No WebSocket/SSE** — long polling only, using TanStack Query patterns
5. **Start with existing SQLC queries** — reuse `ListSpanEventsByTrace` and `ListSpansByTrace`, merge in Go; add filtered queries if performance requires

## Impact

### Affected Specs

| Spec | Type | Description |
|------|------|-------------|
| events-timeline | NEW | Timeline API endpoint, merged view, cursor pagination |
| timeline-ui | NEW | Timeline component in trace detail page |
| long-polling | NEW | Incremental polling for running traces |
| python-sdk-events | NEW | Additional span-level event helper methods |
| contract-alignment | NEW | Cross-layer type/enum alignment pass |

### Affected Code

| Path | Change |
|------|--------|
| `contracts/openapi/openapi.yaml` | ADD: `/api/traces/{id}/events` endpoint, `TimelineResponse`, `TimelineEvent` schemas |
| `internal/api/server_gen.go` | REGENERATED via `make generate` |
| `internal/api/server.go` | ADD: `GetTraceEvents` handler |
| `internal/api/mapper.go` | ADD: timeline event mapping helpers |
| `web/src/api/client.ts` | ADD: timeline types and fetch function |
| `web/src/pages/TraceDetailPage.tsx` | MODIFY: add timeline tab/section |
| `web/src/components/Timeline.tsx` | NEW: timeline component(s) |
| `sdks/python/src/continua/span.py` | ADD: `error()`, `exception()`, `metric()` methods |

### Breaking Changes

None — all changes are additive.

### Schema Changes (Database)

None — timeline is computed from existing `span_events` and `spans` tables.

### Schema Changes (OpenAPI)

| Endpoint | Change |
|----------|--------|
| `GET /api/traces/{id}/events` | NEW: Timeline events endpoint with cursor pagination |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Timeline merge performance for large traces | Reuse indexed queries; limit default page size; add SQL-level cursor filtering if profiling warrants |
| Cursor stability across mixed sources | Monotonic cursor key (`created_at`) avoids drift from upsert time adjustments; full refresh on terminal state |
| Polling frequency overwhelming backend | Default 3s interval; only poll when trace is RUNNING; cursor-based incremental fetch |

## Dependencies

No new Go dependencies required. Adding vitest to `web/` for frontend component tests is acceptable if scoped tightly, but not required — type-check verification is sufficient for Phase 4.

## Related Documents

- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
