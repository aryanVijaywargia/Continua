## 1. Test Infrastructure Setup
- [x] 1.1 Add vitest, jsdom, @testing-library/react, @testing-library/jest-dom, @testing-library/user-event as devDependencies in `web/package.json`
- [x] 1.2 Extend `web/vite.config.ts` with a `test` block (environment: jsdom, setup files, globals)
- [x] 1.3 Create `web/src/test/setup.ts` with @testing-library/jest-dom import
- [x] 1.4 Add `"test": "vitest run"` script to `web/package.json`
- [x] 1.5 Verify `pnpm --filter web test` runs and exits cleanly (no tests yet)

## 2. Pure URL Utilities
- [x] 2.1 Create `web/src/utils/tracesSearchParams.ts` with types: `TracesFilterState`, `FetchTracesParams`
- [x] 2.2 Implement `parseTracesParams(searchParams: URLSearchParams): TracesFilterState` with normalization: invalid offset->0; `q` trimmed, whitespace-only->unset; `status` lowercased, alias `error`->`failed`, unknown values->unset (only `running`/`completed`/`failed` accepted after aliasing); `session_id` validated as UUID format, invalid->unset; invalid dates->unset; `has_errors` only on exactly `"true"`; `min_duration_ms` parsed with `Number()`, non-integer/NaN/negative/0->unset
- [x] 2.3 Implement `serializeTracesParams(state: TracesFilterState): URLSearchParams` omitting empty/default values
- [x] 2.4 Implement `buildCanonicalQueryString(state: TracesFilterState): string` for query key + fetch URL
- [x] 2.5 Implement `localDateToISOStart(date: string): string` and `localDateToISOEnd(date: string): string` for date boundary conversion
- [x] 2.6 Implement `deriveActiveChips(state: TracesFilterState): Chip[]` and `clearChip(state: TracesFilterState, chipKey: string): TracesFilterState`
- [x] 2.7 Write unit tests for all pure utilities: parse/serialize round-trip, normalization (whitespace-only q, case-insensitive status, status alias `error`->`failed`, invalid session_id UUID, 0/negative/non-integer min_duration_ms all->unset), date conversion, chip derivation, session_id preservation, and canonicalization equality (equivalent param sets in different input order produce identical canonical strings)

## 3. API Client Update
- [x] 3.1 Define `FetchTracesParams` interface in `web/src/api/client.ts`
- [x] 3.2 Replace `fetchTraces(limit, offset)` with `fetchTraces(params?: FetchTracesParams)` building query string from params
- [x] 3.3 Remove `fetchTracesBySession` function

## 4. URL Hook
- [x] 4.1 Create `web/src/hooks/useTracesSearchParams.ts` wrapping `useSearchParams()` with parse/serialize from utilities
- [x] 4.2 Expose: `filters: TracesFilterState`, `setFilters(updates, mode: 'push' | 'replace')`, `clearAll()`, `clearChip(key)`

## 5. TracesPage Rewrite
- [x] 5.1 Replace `useState(offset)` with `useTracesSearchParams` hook
- [x] 5.2 Update TanStack Query to use `['traces', canonicalQueryString]` as key and `fetchTraces(params)` as queryFn with `placeholderData: keepPreviousData`
- [x] 5.3 Add responsive filter bar: search input (`q`), status select, date range inputs, user ID input, has-errors checkbox, min-duration input (`<input type="number" min="1" step="1">`). All controls MUST have associated `<label>` elements or `aria-label` attributes
- [x] 5.4 Implement 300ms debounce for `q`, `user_id`, `min_duration_ms` with Enter commit; immediate commit for status, has_errors, dates
- [x] 5.4a Rehydrate draft input values (`q`, `user_id`, `min_duration_ms`) from URL when search params change externally (back/forward navigation, chip clear, clearAll)
- [x] 5.5 Add date validation: inline error when `start_time_from > start_time_to`, suppress query until corrected
- [x] 5.6 Add search hint line below search box
- [x] 5.7 Add conditional active-chip row with individual clear and "Clear all" actions
- [x] 5.8 Add error banner above table with error message and retry button; keep controls interactive on error
- [x] 5.9 Differentiate empty states: onboarding (no traces at all) vs. filtered-no-matches (with "Clear all")
- [x] 5.10 Improve row triage: stronger name emphasis, linked session line, error emphasis via `error_count`
- [x] 5.11 Make table container horizontally scrollable on narrow screens
- [x] 5.12 Reset offset to 0 on every committed filter change
- [x] 5.13 Handle stale offset: if fetch returns 0 rows with total > 0, replace offset to last valid page

## 6. Return Navigation
- [x] 6.1 Pass current `/traces?...` location in router state when linking from trace list to detail
- [x] 6.2 Update `TraceDetailPage.tsx` back link to use router state for filtered-list URL when present, falling back to `/traces` on refresh or direct entry

## 7. SessionDetailPage Update
- [x] 7.1 Replace `fetchTracesBySession` import with `fetchTraces`
- [x] 7.2 Update query to call `fetchTraces({ session_id, limit: PAGE_SIZE, offset })`

## 8. Integration Tests
- [x] 8.1 TracesPage integration tests with MemoryRouter + mocked fetch:
  - Initial load from `/traces` with no filters
  - Deep link pre-populates controls and issues exact filtered request
  - Debounced search commits after idle and on Enter
  - Text filters do not commit on blur
  - Filter composition and pagination reset
  - Invalid date range validation
  - Chip clear and "Clear all" behavior
  - `session_id` URL param honored and clearable
  - Error banner and retry
  - Filtered vs unfiltered empty states
  - Rendered row order matches API response order (mock intentionally non-time-sorted search results)
  - Back link from detail restores filtered list URL when router state present; falls back to `/traces` on refresh
  - Keyboard accessibility: all controls and chip dismiss buttons reachable via Tab, visible focus indicators
  - Draft inputs rehydrate correctly on back/forward navigation
  - Malformed session_id in URL is normalized away (no fetch error)
  - Whitespace-only q in URL is normalized away (no ghost chip)

## 9. Verification
- [x] 9.1 `pnpm --filter web type-check` passes
- [x] 9.2 `pnpm --filter web test` passes
- [x] 9.3 `go test ./internal/store -run 'TestSearch_(FindTraceBySpanName|CombinedSearchAndFilter|Pagination|FilterByTimeRange|FilterByHasErrors|FilterByMinDuration)'` passes unchanged
- [ ] 9.4 Manual smoke test: filter bar, chips, pagination, back/forward, responsive layout
