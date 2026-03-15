# Change: Add Session & Scale UX

## Why
The sessions list page lacks search, filtering, sorting, and URL-driven state, making it unusable at scale. Trace reads also lack session identity context (`session_external_id`), forcing users to navigate between pages to correlate traces with sessions. Pagination across the debugger is minimal (no page-size control, no first/last navigation). These gaps become blocking as production usage grows beyond a handful of sessions.

## What Changes
- **Trace API sorting**: `/api/traces` gains `sort_by=started_at` and `sort_dir=asc|desc`. Implemented via two static sqlc queries (`ListTraces` for DESC, `ListTracesAsc` for ASC) since sqlc cannot parameterize SQL keywords. When `q` is present, sort params are ignored and relevance ordering is preserved.
- **Trace session identity**: All trace read paths (`ListTraces`, `ListTracesBySession`, `GetTrace`, trace search) return `session_external_id` via a `LEFT JOIN sessions` on the query layer. API `Trace` and `TraceDetail` schemas gain an optional `session_external_id` field.
- **Session API filtering and sorting**: `/api/sessions` gains `q`, exact-match `user_id`, `sort_by=created_at|trace_count`, and `sort_dir=asc|desc`. (`limit` and `offset` already exist in the live contract and are not new.)
- **Dynamic session query path**: A handwritten dynamic query file (parallel to `search.go`) handles session search (`q` matches `external_id` and `name`), `user_id` exact filtering, and dynamic sorting including `trace_count` via derived-table aggregate join.
- **Session search ranking**: `external_id exact > external_id prefix > all other matches > created_at desc`. No trigram or FTS.
- **Advanced shared pagination**: Replace `PaginationControls` with a shared paginator supporting First/Previous/Next/Last, current page, total pages, page-size selector (UI options: 20/50/100; API keeps existing `maxPageLimit=200`), "showing X-Y of Z", previous-row preservation during refetch, and stale-offset repair.
- **Sessions page URL-driven state**: Rebuild `/sessions` around URL search params (`q`, `user_id`, `sort_by`, `sort_dir`, `limit`, `offset`).
- **Session detail trace UX**: Move session-detail trace table state (`offset`, `limit`, `sort_by`, `sort_dir`) from local `useState` into URL search params on `/sessions/:id`. Add trace sorting, page-size control, keep-previous-data, stale-offset repair, and `returnTo` state links that capture the full URL for exact table state restoration.
- **External-first session identity**: Sessions list, session detail, trace list, and trace detail all render external ID as primary label with UUID as secondary.
- **Session index**: Add `(project_id, created_at DESC, id DESC)` on `sessions`.

## Impact
- Affected specs: `trace-sorting`, `trace-session-identity`, `session-filtering-sorting`, `session-search`, `advanced-pagination`, `session-url-state`, `external-first-identity`
- Affected code:
  - Backend: `contracts/openapi/openapi.yaml`, `db/platform/queries/traces.sql`, `db/platform/queries/sessions.sql`, `internal/store/search.go` (extend), new `internal/store/session_search.go`, `internal/store/store.go`, `internal/api/traces_handlers.go`, `internal/api/sessions_handlers.go`, `internal/api/mapper.go`, new migration for session index
  - Frontend: `web/src/api/client.ts`, `web/src/components/PaginationControls.tsx` (rewrite), `web/src/pages/SessionsPage.tsx` (rewrite), `web/src/pages/SessionDetailPage.tsx`, `web/src/pages/TracesPage.tsx`, `web/src/pages/TraceDetailPage.tsx`
- No new dependencies
