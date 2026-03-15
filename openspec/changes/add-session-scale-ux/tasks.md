## 1. Contract and codegen (foundation for all backend/frontend work)

- [x] 1.1 Add `sort_by` (enum: `started_at`) and `sort_dir` (enum: `asc`, `desc`) query params to `GET /api/traces` in `contracts/openapi/openapi.yaml`
- [x] 1.2 Add optional `session_external_id` string field to `Trace` and `TraceDetail` schemas in `contracts/openapi/openapi.yaml`
- [x] 1.3 Add `q`, `user_id`, `sort_by` (enum: `created_at`, `trace_count`), `sort_dir` (enum: `asc`, `desc`) query params to `GET /api/sessions` in `contracts/openapi/openapi.yaml`
- [x] 1.4 Run `make generate` and verify generated Go server and TypeScript types include new params and fields
- [x] 1.5 Add `FetchSessionsParams` interface to `web/src/api/client.ts` mirroring the object-parameter pattern used by `FetchTracesParams`

## 2. Database: session index migration

- [x] 2.1 Create migration adding index `(project_id, created_at DESC, id DESC)` on `sessions`
- [x] 2.2 Verify migration applies cleanly and existing `(project_id, user_id)` index is left untouched

## 3. Trace session identity (atomic cross-layer change)

- [x] 3.1 Update sqlc trace queries (`ListTraces`, `ListTracesBySession`, `GetTrace`) to `LEFT JOIN sessions` and select `sessions.external_id AS session_external_id`
- [x] 3.2 Run `make generate` to produce updated row types with `session_external_id`
- [x] 3.3 Add a unified trace read model in the store layer carrying trace data plus optional `session_external_id`
- [x] 3.4 Extend the handwritten trace search path (`search.go`) to select `session_external_id` from the same join
- [x] 3.5 Update store wrapper methods to populate the unified read model from both sqlc and search paths
- [x] 3.6 Update `traceToAPI` and `traceDetailToAPI` in `mapper.go` to map `session_external_id`
- [x] 3.7 Add/update trace read tests to verify `session_external_id` is present when session exists and absent when not

## 4. Trace sorting (backend)

- [x] 4.1 Add `ListTracesAsc` sqlc query: identical to `ListTraces` but with `ORDER BY COALESCE(start_time, server_received_at) ASC`; add matching `ListTracesBySessionAsc` query
- [x] 4.2 Run `make generate` to produce the new query functions
- [x] 4.3 Add store wrapper that selects the correct query (`ListTraces` vs `ListTracesAsc`, `ListTracesBySession` vs `ListTracesBySessionAsc`) based on a `sortDir` parameter
- [x] 4.4 Update trace handlers to parse `sort_by` and `sort_dir` params, ignore them when `q` is present, and pass through to store
- [x] 4.5 Add handler/store tests for `started_at asc`, `started_at desc`, default order, and search-overrides-sort behavior

## 5. Dynamic session query path (backend)

- [x] 5.1 Create `internal/store/session_search.go` with dynamic query builder supporting `q`, `user_id`, `sort_by`, `sort_dir`, `limit`, `offset`
- [x] 5.2 Implement session search ranking with CASE expression: exact `external_id` match > prefix match > other ILIKE matches > `created_at DESC` tiebreaker
- [x] 5.3 Implement `trace_count` sorting via derived-table aggregate join with `COALESCE(tc.cnt, 0)`
- [x] 5.4 Implement total count query that applies the same filters for accurate pagination totals
- [x] 5.5 Update session handlers to parse new params and route to dynamic query path when any non-default param is present
- [x] 5.6 Add store tests: default order, `created_at asc|desc`, `trace_count asc|desc`, `q` exact/prefix ranking on `external_id`, `q + user_id` combination, project scoping, stable pagination with no duplicates

## 6. Session API handler integration tests

- [x] 6.1 Test new session params parse correctly and return expected results
- [x] 6.2 Test search-active requests ignore sort params
- [x] 6.3 Test session totals stay correct after filtering
- [x] 6.4 Test cross-project session access returns 404

## 7. Advanced shared pagination component (frontend, prerequisite for list page changes)

- [x] 7.1 Rewrite `PaginationControls` with First/Previous/Next/Last buttons, current page, total pages, page-size selector (20/50/100), and "showing X-Y of Z" label
- [x] 7.2 Implement stale-offset repair: when total shrinks below current offset, auto-repair to last valid page
- [x] 7.3 Add Vitest tests for pagination math, button disabled states, page-size change resetting offset, and stale-offset repair
- [x] 7.4 Integrate updated paginator into `/traces` page (replacing current PaginationControls)

## 8. Traces page sorting and session identity (frontend)

- [x] 8.1 Add sortable `Started` column header to traces list with sort direction toggle updating URL params
- [x] 8.2 Disable sort header when search is active (visually muted, non-clickable)
- [x] 8.3 Display `session_external_id` as primary session label on trace rows (external ID first, UUID as secondary)
- [x] 8.4 Preserve existing `keepPreviousData` behavior on traces list while integrating sorting and pagination upgrades
- [x] 8.5 Add Vitest tests for sort header toggle, search-disables-sort, and session external ID rendering

## 9. Sessions page URL-driven state (frontend)

- [x] 9.1 Create `useSessionsSearchParams` hook managing `q`, `user_id`, `sort_by`, `sort_dir`, `limit`, `offset` from URL
- [x] 9.2 Rebuild `SessionsPage` around URL search params with filter/sort/page controls resetting offset to 0
- [x] 9.3 Implement malformed param normalization (strip invalid values, replace in URL)
- [x] 9.4 Add sortable `Traces` and `Created` column headers with search-active disabling
- [x] 9.5 Add search input and user_id filter input
- [x] 9.6 Wire `FetchSessionsParams` to TanStack Query with URL-derived query key
- [x] 9.7 Add keep-previous-data behavior during refetch
- [x] 9.8 Add Vitest tests: deep-link rehydration, sort/page-size changes reset offset, search strips sort from URL, stale-offset repair

## 10. External-first session identity (frontend)

- [x] 10.1 Sessions list: render external ID as primary clickable label, UUID as secondary muted text
- [x] 10.2 Session detail header: external ID first, UUID second, both copyable
- [x] 10.3 Trace detail session field: external ID first, UUID available for copy/debugging
- [x] 10.4 Add Vitest tests for external-first rendering on sessions list, session detail, trace list, and trace detail

## 11. Session detail URL-driven trace table UX (frontend)

- [x] 11.1 Move session-detail trace table state (`offset`, `limit`, `sort_by`, `sort_dir`) from local `useState` into URL search params on the `/sessions/:id` route (e.g., `/sessions/:id?sort_by=started_at&sort_dir=asc&limit=50&offset=20`)
- [x] 11.2 Add trace sorting (by `started_at`) to session detail trace table via URL params
- [x] 11.3 Integrate advanced paginator with page-size control, wired to URL params
- [x] 11.4 Add keep-previous-data and stale-offset repair
- [x] 11.5 Build `returnTo` from the full current session-detail URL including search params: `state={{ returnTo: currentSessionDetailUrl }}`
- [x] 11.6 Extend trace detail `returnTo` validator to accept paths starting with `/sessions/`
- [x] 11.7 Add Vitest tests: trace sorting toggle, URL param round-trip, returnTo restores exact table state, no trace filter bar present

## 12. End-to-end validation

- [x] 12.1 Run `make generate` and verify no drift
- [x] 12.2 Run `make test` (backend) and `pnpm --filter web test` (frontend) with all new and existing tests passing
- [x] 12.3 Run `make lint` with no new warnings
