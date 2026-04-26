# Change: Add Session-Scoped Trace Comparison

## Why

Multi-trace sessions need a way to compare two executions side-by-side so users can understand what changed between runs — different decisions, new effects, shifted timing, added or removed spans. Today the session detail page shows individual trace summaries but offers no structured diff. Phase 8 adds a read-only comparison surface that pairs spans and semantic events across two terminal traces within the same session, with deterministic server-side diff output.

## What Changes

- **New REST endpoint** `GET /api/sessions/{id}/compare` accepts `baseline_trace_id` and `candidate_trace_id` query params and returns a full comparison payload including matched/unmatched span rows with inline semantic detail groups.
- **New store builder** in `internal/store` assembles the comparison using scoped sqlc reads (session/trace validation, span fetch, semantic event fetch) plus deterministic Go-side matching and diff logic.
- **New compare page** at `/sessions/:id/compare` renders overview + ordered span diff with inline semantic expansion, and preserves the selected pair when users navigate back to the parent session page.
- **Session detail page extended** with baseline/candidate selection via URL params, a compare bar, bounded selected-trace hydration via existing trace lookups when selections fall outside the currently loaded narrative/table slice, and actions on both storyline cards and trace table rows.
- **New OpenAPI schemas** for the comparison response, span diff rows, semantic detail groups, and error responses.

## V1 Scope Limits

- Same-session only; both traces must be terminal — eligible when `NormalizeTraceStatus(status)` returns `completed` or `failed`, rejected when it returns `running`.
- Read-only; no mutations, no replay, no engine involvement.
- Full response in one request; no pagination.
- Payload ceiling: 500 spans per trace, 1,000 semantic events (decision/effect/wait) per trace — returns `422 comparison_too_large` when exceeded.
- Only spans receive heuristic structural matching. Semantic events pair by stable IDs (effect_id, wait_id) or, for decisions, by unique normalized question string within matched span pairs — see design.md for full rules.
- No full payload diff (span input/output); only precomputed `changed_fields` per row.
- No dual-workspace overlay or merged tree visualization.
- No live polling or running-trace compare mode.

## Impact

- Affected specs: `session-trace-comparison` (new capability)
- Affected code:
  - `contracts/openapi/openapi.yaml` — new path + schemas
  - `internal/api/sessions_handlers.go` — new handler
  - `internal/api/mapper.go` — comparison mapping
  - `internal/store/` — new comparison builder + queries
  - `db/platform/queries/` — new SQL queries (no migrations)
  - `web/src/pages/SessionDetailPage.tsx` — selection UI + compare bar
  - `web/src/pages/SessionComparePage.tsx` — new page
  - `web/src/api/client.ts` — new types + fetch function
  - `web/src/App.tsx` — new route

## Dependencies

- `add-session-narrative` (Phase 7) is treated as present in the live repo. The compare entry point reuses the narrative storyline cards and session detail page structure.
