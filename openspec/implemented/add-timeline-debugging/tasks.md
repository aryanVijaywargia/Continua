## 0. Repo Discovery
- [x] 0.1 Document existing span_events schema, spans lifecycle fields, event queries, store patterns, handler patterns, mapper logic, SDK helpers, trace detail UI structure, and project scoping
- [x] 0.2 Write `docs/phase4/spec-0-discovery.md`

## 1. Events Timeline API
- [x] 1.1 Add `GET /api/traces/{id}/events` endpoint to `contracts/openapi/openapi.yaml` with `TimelineResponse`, `TimelineEvent`, `TimelineEventType`, `TimelineEventSource` schemas and `after`/`limit` query params
- [x] 1.2 Run `make generate` and verify clean compilation
- [x] 1.3 Implement `GetTraceEvents` handler in `internal/api/server.go`: fetch events + spans, generate synthetic lifecycle markers, merge chronologically, apply cursor pagination, return trace_status
- [x] 1.4 Add timeline mapping helpers in `internal/api/mapper.go`
- [x] 1.5 Add backend tests for: merged timeline, ordering, cursor pagination (no duplicates), invalid cursor returns 400, orphan events included, project scoping, trace_status propagation
- [x] 1.6 Write `docs/phase4/spec-1-timeline-api.md`

## 2. Timeline UI
- [x] 2.1 Add `TimelineEvent` type and `fetchTimelineEvents` function to `web/src/api/client.ts`
- [x] 2.2 Create timeline component(s) under `web/src/components/` with visual distinction for explicit vs synthetic events, error highlighting, expandable payloads
- [x] 2.3 Integrate timeline into `web/src/pages/TraceDetailPage.tsx` as tab or section
- [x] 2.4 Verify frontend via `make type-check`; add vitest component tests if test tooling exists or can be added with minimal scope, otherwise verify manually
- [x] 2.5 Write `docs/phase4/spec-2-timeline-ui.md`

## 3. Long Polling
- [x] 3.1 Implement cursor-based incremental polling in trace detail page using TanStack Query refetchInterval
- [x] 3.2 Poll only when trace_status is RUNNING; on terminal state, do one cursorless full refresh then stop
- [x] 3.3 Merge-sort new events into existing timeline by display timestamp, deduplicating by event ID; show live indicator
- [ ] 3.4 Verify polling behavior via `make type-check` and manual testing (start/stop, no duplicates, full refresh on terminal)
- [x] 3.5 Write `docs/phase4/spec-3-long-polling.md`

## 4. Python SDK Event Helpers
- [x] 4.1 Add `span.error(message, payload)` method emitting event_type="error" with level="error"
- [x] 4.2 Add `span.exception(exc, payload)` method emitting event_type="exception" with level="error" and exception details in payload
- [x] 4.3 Add `span.metric(name, value, unit, payload)` method emitting event_type="metric" with metric data in payload
- [x] 4.4 Add SDK tests for all new helpers (payload correctness, level, batching); run with `cd sdks/python && uv run pytest -q`
- [x] 4.5 Add or update example(s) demonstrating event helpers
- [x] 4.6 Write `docs/phase4/spec-4-python-sdk.md`

## 5. Contract Alignment
- [x] 5.1 Verify `make generate` succeeds with all OpenAPI changes
- [x] 5.2 Verify backend handler response shapes match OpenAPI contract
- [x] 5.3 Align manual types in `web/src/api/client.ts` with timeline and existing contracts
- [x] 5.4 Verify Python SDK generated types are current
- [x] 5.5 Run full test suite: `make test` (Go + JS) and `cd sdks/python && uv run pytest -q` (Python)
- [x] 5.6 Write `docs/phase4/spec-5-alignment.md`
- [x] 5.7 Write `docs/phase4/REPORT.md`
