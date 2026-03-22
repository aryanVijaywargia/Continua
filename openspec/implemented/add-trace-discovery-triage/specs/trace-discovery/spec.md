## ADDED Requirements

### Requirement: Trace List Filtering

The traces page SHALL expose six visible filter controls: text search (`q`), status, date range (`start_time_from`, `start_time_to`), user ID, has-errors toggle, and minimum duration (`min_duration_ms`, rendered as `<input type="number" min="1" step="1">`, positive integers only, 0 and non-integers treated as unset). `session_id` is a URL-only supported filter with no visible control — it is accepted from the URL, applied to the query, surfaced as a chip, and clearable. Filter state SHALL be driven entirely by URL search params so that filtered views are bookmarkable and shareable. The canonical URL form for `status` SHALL be lowercase (`running`, `completed`, `failed`); the parser SHALL accept case-insensitive input and normalize to lowercase.

#### Scenario: User applies a text search filter
- **WHEN** the user types a search query into the search input and the 300ms debounce expires or the user presses Enter
- **THEN** the `q` parameter is added to the URL, the traces list is re-fetched with the `q` filter, offset resets to 0, and a search chip appears in the active-chip row

#### Scenario: User applies a status filter
- **WHEN** the user selects a status value from the status dropdown
- **THEN** the `status` parameter is added to the URL immediately, the traces list is re-fetched, offset resets to 0, and a status chip appears

#### Scenario: User applies a date range filter
- **WHEN** the user sets a start date and/or end date using native date inputs
- **THEN** the dates are converted to ISO boundary timestamps (start-of-day for `from`, end-of-day 23:59:59.999 for `to`), added to the URL, and the traces list is re-fetched

#### Scenario: Invalid date range
- **WHEN** the committed `start_time_from` is later than `start_time_to`
- **THEN** an inline validation error is shown and the traces query is suppressed until corrected

#### Scenario: User applies has-errors filter
- **WHEN** the user activates the has-errors toggle
- **THEN** the `has_errors=true` parameter is added to the URL and the traces list is re-fetched showing only traces with errors

#### Scenario: User applies min-duration filter
- **WHEN** the user enters a positive integer minimum duration value and the 300ms debounce expires or Enter is pressed
- **THEN** the `min_duration_ms` parameter is added to the URL and the traces list is re-fetched

#### Scenario: Min-duration value of 0 is treated as unset
- **WHEN** the user enters 0 as the minimum duration value
- **THEN** the `min_duration_ms` parameter is removed from the URL and no duration filter is applied

#### Scenario: Non-integer min-duration is treated as unset
- **WHEN** the URL contains a non-integer `min_duration_ms` value (e.g., `1.9`, `abc`)
- **THEN** the `min_duration_ms` parameter is removed from the URL and no duration filter is applied

#### Scenario: Session ID filter from URL (URL-only supported filter)
- **WHEN** the URL contains a `session_id` parameter with a valid UUID value (e.g., from a deep link or session page navigation)
- **THEN** the filter is applied, a session chip is shown, and the chip can be cleared

#### Scenario: Invalid session_id in URL is normalized away
- **WHEN** the URL contains a `session_id` parameter with an invalid (non-UUID) value
- **THEN** the `session_id` parameter is silently removed from the URL via replace and no session filter is applied

#### Scenario: Whitespace-only search query is normalized away
- **WHEN** the URL contains a `q` parameter that is empty or whitespace-only after trimming
- **THEN** the `q` parameter is removed from the URL, no search chip is shown, and no search filter is applied

#### Scenario: Status case normalization
- **WHEN** the URL contains a `status` parameter with a valid value in any case (e.g., `RUNNING`, `Running`, `running`)
- **THEN** the value is normalized to lowercase in the URL and the status filter is applied

#### Scenario: Status alias normalization
- **WHEN** the URL contains `status=error` (a backend-accepted alias for `failed`)
- **THEN** the value is canonicalized to `status=failed` in the URL and the failed status filter is applied

#### Scenario: Text inputs do not commit on blur
- **WHEN** a user types into `q`, `user_id`, or `min_duration_ms` and then tabs or clicks away without pressing Enter and before the debounce expires
- **THEN** the filter value is NOT committed to the URL

### Requirement: Active Filter Chips

The traces page SHALL display active filter chips only when at least one filter parameter is set. Each chip SHALL have an individual clear action. A "Clear all" action SHALL remove every filter parameter including `session_id` and reset `offset` to 0.

#### Scenario: Chips appear when filters are active
- **WHEN** one or more filter parameters are set in the URL
- **THEN** a chip row appears below the filter bar showing one chip per active filter, each with a clear button

#### Scenario: Clear individual chip
- **WHEN** the user clicks the clear button on a single chip
- **THEN** only that filter parameter is removed from the URL, the traces list re-fetches, and offset resets to 0

#### Scenario: Clear all filters
- **WHEN** the user clicks "Clear all"
- **THEN** all filter parameters including `session_id` and `offset` are removed from the URL and the traces list shows unfiltered results

### Requirement: Search Hint

The traces page SHALL display a concise search hint near the search input describing the search scope.

#### Scenario: Hint visibility
- **WHEN** the traces page is rendered
- **THEN** a short hint is displayed: "Search names, user IDs, and matching span names."

### Requirement: URL-Driven Pagination and History

The traces page SHALL drive pagination from the `offset` URL parameter. User-initiated filter and pagination changes SHALL push browser history entries. Internal normalization (e.g., invalid param cleanup, stale-offset repair) SHALL use `replace` to avoid junk history entries.

#### Scenario: Browser back/forward restores filtered state
- **WHEN** the user navigates back or forward in browser history
- **THEN** the filter controls, chip row, and fetched results reflect the URL state at that history entry

#### Scenario: Stale offset repair
- **WHEN** the current offset is out of range and a fetch returns 0 rows with `total > 0`
- **THEN** the offset is replaced (not pushed) to the last valid page and the list re-fetches

#### Scenario: Default state
- **WHEN** the user navigates to `/traces` with no query parameters
- **THEN** the page shows all traces, first page, limit 20, with no active chips

### Requirement: Differentiated Empty States

The traces page SHALL distinguish between having no traces at all and having no matches for active filters.

#### Scenario: No traces exist
- **WHEN** the fetch returns 0 total traces and no filters are active
- **THEN** an onboarding-style empty state is shown

#### Scenario: No filter matches
- **WHEN** the fetch returns 0 traces but filters are active
- **THEN** a filtered empty state is shown with a "Clear all" action

### Requirement: Error Handling with Interactive Controls

The traces page SHALL show an error banner above the table on fetch failure with an error message and retry action. Filter controls SHALL remain interactive during error state.

#### Scenario: Fetch failure
- **WHEN** the traces fetch fails
- **THEN** an error banner is displayed above the table area, a retry button is available, and all filter controls remain interactive

### Requirement: Trace Row Triage Improvements

The traces list rows SHALL provide improved visual triage using existing list fields only: stronger name emphasis, session shown as a secondary linked line when present, and clearer failed/error emphasis via `error_count`.

#### Scenario: Row with errors
- **WHEN** a trace has `error_count > 0`
- **THEN** the error count is visually emphasized in the row

#### Scenario: Row with session
- **WHEN** a trace has a `session_id`
- **THEN** the session is shown as a secondary linked line beneath the trace name

### Requirement: Responsive Table Layout

The traces list table container SHALL be horizontally scrollable on narrow screens instead of relying on fixed-width columns.

#### Scenario: Narrow viewport
- **WHEN** the viewport is narrower than the table's minimum width
- **THEN** the table container scrolls horizontally

### Requirement: Filtered-List Return Navigation

Links from the traces list to trace detail SHALL pass the current `/traces?...` location in router state. The trace detail page SHALL use that state for its back link when present, falling back to `/traces` when router state is unavailable (e.g., after page refresh or direct URL entry).

#### Scenario: Return to filtered list via router state
- **WHEN** the user clicks the back link on a trace detail page after arriving from a filtered traces list in the same session
- **THEN** the browser navigates to the filtered URL that was active when the user left the list

#### Scenario: Return to traces list without router state
- **WHEN** the user loads a trace detail page directly (via bookmark, refresh, or new tab) and clicks the back link
- **THEN** the browser navigates to `/traces` with no filters

### Requirement: Data Freshness During Refetch

The traces page SHALL keep previous rows visible while a new filter or page request is in flight, using TanStack Query's `keepPreviousData` behavior.

#### Scenario: Filter change during loading
- **WHEN** the user changes a filter and a new request is in flight
- **THEN** the previous rows remain visible until the new data arrives

### Requirement: Filter Control Accessibility

All filter controls, chip dismiss buttons, and the "Clear all" button SHALL be keyboard-reachable via Tab. Each control SHALL have an associated `<label>` element or `aria-label` attribute. Interactive elements SHALL have visible focus indicators.

#### Scenario: Keyboard navigation through filters
- **WHEN** the user navigates the filter bar using the Tab key
- **THEN** every filter control, chip dismiss button, and "Clear all" button receives focus in a logical order with a visible focus indicator

### Requirement: Draft State Rehydration on URL Change

When URL search params change due to back/forward navigation, chip clear, or "Clear all," the draft input values for debounced controls (`q`, `user_id`, `min_duration_ms`) SHALL be rehydrated from the new URL state so that displayed input values match the active query.

#### Scenario: Back navigation updates draft inputs
- **WHEN** the user navigates back in browser history to a URL with different filter values
- **THEN** the text input controls display the values from the restored URL, not stale draft values

### Requirement: Server-Ordered Results

The traces page SHALL render traces in the exact order returned by the API and SHALL NOT apply client-side re-sorting. This preserves backend relevance ranking when `q` is present and default time ordering otherwise.

#### Scenario: Search results preserve relevance ordering
- **WHEN** the user applies a text search filter (`q`) and the backend returns results in relevance-ranked order
- **THEN** the traces list renders rows in the same order as the API response without re-sorting

#### Scenario: Unfiltered results preserve API ordering
- **WHEN** no `q` parameter is set and the backend returns results in default time order
- **THEN** the traces list renders rows in the same order as the API response without re-sorting

### Requirement: Unified Trace Fetch API

The web client SHALL provide a single `fetchTraces(params?: FetchTracesParams)` function that accepts all filter parameters. The `fetchTracesBySession` function SHALL be removed; callers SHALL use `fetchTraces({ session_id })` instead.

#### Scenario: Session detail page fetches traces
- **WHEN** `SessionDetailPage` needs traces for a specific session
- **THEN** it calls `fetchTraces({ session_id, limit, offset })` instead of the removed `fetchTracesBySession`
