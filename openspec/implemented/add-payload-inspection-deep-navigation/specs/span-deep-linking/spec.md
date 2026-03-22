# Span Deep-Linking

URL-driven selected span state on the trace detail page using `?span=<external-span-id>`.

## ADDED Requirements

### Requirement: URL Span Parameter Parsing

The URL utility layer (`traceDetailSearchParams.ts`) MUST parse the `span` query parameter as a raw string. It MUST NOT perform semantic validation (e.g., checking whether the span exists in the data). Semantic validation is the responsibility of `TraceDetailPage` after span data has loaded.

#### Scenario: Parse present span parameter

Given URL `/traces/123?span=abc`
When `parseSpanParam(searchParams)` is called
Then it returns `"abc"`

#### Scenario: Parse missing span parameter

Given URL `/traces/123` with no `span` parameter
When `parseSpanParam(searchParams)` is called
Then it returns `null`

#### Scenario: Parse empty span parameter

Given URL `/traces/123?span=`
When `parseSpanParam(searchParams)` is called
Then it returns `null` (empty string normalized to absent)

#### Scenario: Unrelated params preserved on write

Given URL `/traces/123?span=abc&debug=true`
When `serializeSpanParam(searchParams, "def")` is called
Then the result contains `span=def` and `debug=true`

### Requirement: Semantic Span Validation

`TraceDetailPage` MUST validate the parsed `span` value against the loaded `spanIndex`. Unknown spans MUST be cleaned up only after span data is available.

#### Scenario: Valid span on load

Given URL `/traces/123?span=abc` and span `abc` exists in the fetched span data
When the trace detail page renders with span data
Then span `abc` is selected and `userHasSelected` is set to `true`

#### Scenario: No span parameter on load

Given URL `/traces/123` with no `span` parameter
When the trace detail page renders
Then no URL-driven selection occurs and Phase 7 auto-selection runs if applicable

#### Scenario: Unknown span on load

Given URL `/traces/123?span=nonexistent` where `nonexistent` is not in the span data
When the trace detail page renders with span data available
Then `TraceDetailPage` removes `?span=` from the URL with `replace`, sets `userHasSelected` to `false`, and Phase 7 auto-selection runs

#### Scenario: Unrelated params preserved during cleanup

Given URL `/traces/123?span=nonexistent&debug=true`
When `TraceDetailPage` removes the unknown span
Then `debug=true` is preserved in the resulting URL

### Requirement: URL Span Parameter Writing

The system MUST update the `span` query parameter when the user explicitly selects a span. Automatic selections MUST NOT write to the URL.

#### Scenario: User selects span via tree

Given the user clicks a span in the span tree
When the selection callback fires
Then the URL is updated to include `?span=<selected-external-id>` using `replace`

#### Scenario: Unrelated params preserved on user selection

Given the current URL is `/traces/123?debug=true` and the user clicks a span
When the selection callback writes `?span=abc`
Then the resulting URL is `/traces/123?debug=true&span=abc` (unrelated params preserved)

#### Scenario: User selects span via breadcrumb

Given the user clicks an ancestor in the span breadcrumb
When the selection callback fires
Then the URL is updated to include `?span=<selected-external-id>` using `replace`

#### Scenario: Failure-first auto-selection

Given Phase 7 auto-selects the primary failed span on page load
When the auto-selection occurs
Then the URL is NOT modified (no `?span=` added)

#### Scenario: Copy Trace URL with auto-selected span

Given Phase 7 auto-selected span `abc` (URL does not contain `?span=`)
When the user clicks "Copy Trace URL"
Then the copied URL includes `?span=abc` (the effective selected span)

### Requirement: Reactive URL Span Changes

The system MUST react to `?span=` changes while the trace detail component remains mounted (e.g., browser back/forward or manual URL edits). It MUST NOT only read the span parameter on initial mount.

#### Scenario: Browser back changes span param

Given the user is viewing `/traces/123?span=abc` and browser back navigates to `/traces/123?span=def` (same trace, different span)
When the URL changes while the component stays mounted
Then the selection updates to span `def` (if it exists in `spanIndex`)

#### Scenario: Browser back removes span param

Given the user is viewing `/traces/123?span=abc` and browser back navigates to `/traces/123` (no span param)
When the URL changes while the component stays mounted
Then the selection clears, `userHasSelected` resets to `false`, and Phase 7 auto-selection re-runs

#### Scenario: Manual URL edit

Given the user manually edits the URL bar to change `?span=abc` to `?span=xyz`
When the URL change is committed
Then the selection updates to span `xyz` if it exists, or clears with fallback if it does not

### Requirement: History Behavior

Internal span changes MUST use `replace` to avoid polluting browser history with span-by-span navigation.

#### Scenario: Back button after span navigation

Given the user navigated from the trace list to trace detail, then selected 3 different spans
When the user presses the browser back button
Then the browser returns to the trace list (not to a prior span selection)

#### Scenario: External deep link

Given a user opens `/traces/123?span=abc` from a shared link
When the page loads
Then span `abc` is selected and the user can press back to return to their previous page

### Requirement: Stale Span Fallback

When the selected span disappears from the data (e.g., after a polling refresh), the system MUST fall back gracefully.

#### Scenario: Selected span disappears

Given span `abc` is selected via URL and a data refresh no longer includes span `abc`
When the refresh completes
Then the selection clears, `userHasSelected` resets to `false`, `?span=` is removed from the URL, and Phase 7 auto-selection re-runs

#### Scenario: Span remains across polling

Given span `abc` is selected via URL and a polling refresh still includes span `abc`
When the refresh completes
Then span `abc` remains selected and the URL is unchanged

### Requirement: No Refetch on Span Change

Changing the `?span=` parameter MUST NOT trigger React Query refetches. Query keys remain trace-id-based.

#### Scenario: Span parameter change

Given the trace detail page is loaded with trace data cached
When the user selects a different span (updating `?span=`)
Then no new API requests are made for trace or span data
