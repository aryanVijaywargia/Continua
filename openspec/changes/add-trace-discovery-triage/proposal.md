# Change: Add Trace Discovery & Triage (Phase 6)

## Why

The backend already supports rich filtering (`q`, `status`, `start_time_from`, `start_time_to`, `user_id`, `session_id`, `has_errors`, `min_duration_ms`) but the web UI exposes none of it. Users must scan an unfiltered, unpaginated-by-URL list to find traces, with no way to bookmark or share a filtered view.

## What Changes

- **Web client API**: Replace `fetchTraces(limit, offset)` with `fetchTraces(params?: FetchTracesParams)` that forwards all supported filter parameters. Remove `fetchTracesBySession`; `SessionDetailPage` calls `fetchTraces({ session_id })` instead.
- **URL/state architecture**: New `tracesSearchParams.ts` utilities and `useTracesSearchParams` hook drive all filter state from URL search params. Back/forward navigation restores prior filter states.
- **Filter bar UI**: Responsive filter bar on `TracesPage` with six visible controls: search (`q`), status select, date range (`<input type="date">`), user ID, has-errors toggle, and min-duration input (`<input type="number" min="1" step="1">`, positive integers only, 0 and non-integers treated as unset). Text inputs debounce 300ms; selects/dates commit immediately. `session_id` is a URL-only supported filter with no dedicated control — it is accepted from the URL, applied to the query, surfaced as a chip, and clearable.
- **Active filter chips**: Rendered only when at least one filter is active. Each chip has a clear action; "Clear all" resets everything including `session_id` and `offset`.
- **Search hint**: Single concise line beneath the search box.
- **Empty & error states**: Differentiated onboarding vs. filtered-no-matches empty states. Error banner with retry; controls stay interactive on error.
- **Row triage improvements**: Stronger name emphasis, linked session line, clearer error emphasis via `error_count`. No schema expansion.
- **Responsive table**: Horizontal scroll on narrow screens.
- **Return navigation**: Trace list links pass current filtered URL in router state; detail page uses it for back-link when present, falling back to `/traces` on refresh or direct entry.
- **Test tooling**: Add vitest, jsdom, @testing-library/{react,jest-dom,user-event} to `web/`; extend `vite.config.ts` with test block.

## Impact

- Affected specs: `trace-discovery` (new capability)
- Affected code:
  - `web/src/api/client.ts` (modified)
  - `web/src/utils/tracesSearchParams.ts` (new)
  - `web/src/hooks/useTracesSearchParams.ts` (new)
  - `web/src/pages/TracesPage.tsx` (modified)
  - `web/src/pages/TraceDetailPage.tsx` (modified)
  - `web/src/pages/SessionDetailPage.tsx` (modified)
  - `web/vite.config.ts` (modified)
  - `web/package.json` (modified)
  - `web/src/test/setup.ts` (new)
- No backend, OpenAPI, Go handler, SQL, or migration changes.
