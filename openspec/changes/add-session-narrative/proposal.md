# Change: Add Session Narrative

## Why

The session detail page currently shows a flat, paginated trace table with no high-level summary or chronological storyline. Users debugging multi-trace agent sessions need to quickly understand the session shape, aggregate cost/token usage, and trace-to-trace flow before diving into individual traces. A lightweight narrative endpoint and UI surface fills this gap without adding session lifecycle control or new routes.

## What Changes

- **New API endpoint**: `GET /api/sessions/{id}/narrative` returns a capped (100 traces), chronologically ordered narrative with session-level summary aggregates, per-trace activity timestamps, semantic event snapshots, and lineage classification.
- **New OpenAPI schemas**: `SessionNarrativeResponse`, `SessionNarrativeSummary`, `SessionNarrativeTrace`, `SessionNarrativeLineage` (reuses existing `TimelineEvent`, with docs clarifying that `SessionNarrativeTrace.trace_id` is external while nested `semantic_events[].trace_id` remains the internal trace UUID from `TimelineEvent`).
- **New store method**: `BuildSessionNarrative(ctx, projectID, sessionID, limit)` with an up-to-3 SQL round-trip plan. The successful `200` path uses 3 queries by default; missing/cross-project requests may return after Query 1. Query 1 is anchored on `sessions` so zero-trace sessions return `200` while missing/cross-project sessions return `404` without an extra existence lookup. Summary status counts follow the same normalization semantics as `mapTraceStatus` (`completed|ok`, `failed|error|cancelled`, everything else => running`). Note: the session summary `last_activity_at` is trace-level only (not span/event) for cost reasons; per-trace `latest_activity_at` is the authoritative activity timestamp.
- **Lineage resolution**: inference-first lineage (clean-gap predecessor rule), plus required explicit metadata lineage (`metadata.__continua_lineage`) as the cut-safe final slice.
- **Frontend**: new React Query on `SessionDetailPage` for the narrative, rendered as summary + storyline sections above the existing trace table. The UI reuses existing inline loading/error containment and trace-detail `returnTo` patterns, labels lineage coverage as applying to the shown narrative only, and uses a compact zero-trace narrative placeholder instead of a second full-size empty-state card. Polling continues while normalized running traces exist.

## Impact

- Affected specs: `session-narrative` (new capability)
- Affected code:
  - `contracts/openapi/openapi.yaml` (new path + 4 schemas)
  - `internal/api/sessions_handlers.go` (new handler)
  - `internal/api/mapper.go` (narrative mapping)
  - `internal/store/` (new `BuildSessionNarrative` method + SQL)
  - `web/src/api/client.ts` (new fetch function)
  - `web/src/pages/SessionDetailPage.tsx` (summary + storyline sections)
- No migration needed: reads existing traces/spans/span_events tables; explicit lineage uses existing `traces.metadata` JSONB column.
- No breaking changes to existing endpoints or UI behavior.
