## Context

Phase 6 is frontend-only. The backend (`listTraces` handler) already accepts all filter query parameters defined in the OpenAPI contract. This change surfaces those filters in the web UI without touching Go, SQL, or OpenAPI.

Key stakeholders: frontend consumers of the traces list, users debugging AI agents.

## Goals / Non-Goals

- Goals:
  - Expose all existing backend filter capabilities in the web UI
  - Make filtered views bookmarkable and shareable via URL
  - Improve trace list triage with visual emphasis and error highlighting
  - Establish web test infrastructure (vitest + testing-library)

- Non-Goals:
  - Backend changes (no OpenAPI, handler, or SQL modifications)
  - Client-side sorting or ranking
  - Payload/content search
  - Collapsible filter drawer or mobile-specific layout beyond responsive stacking
  - Expanding the list schema with `user_id`, `tags`, `environment`, or `release` fields

## Decisions

### URL as single source of truth for filter state

- Decision: All filter state lives in URL search params. React state is only for local draft values (pre-commit debounce).
- Why: Enables bookmarking, sharing, and back/forward navigation. Avoids state synchronization bugs.
- Alternatives considered: React state with URL sync — rejected because it creates two sources of truth.

### Pure utilities + thin hook split

- Decision: `tracesSearchParams.ts` contains pure parse/normalize/serialize functions; `useTracesSearchParams.ts` is a thin React wrapper.
- Why: Pure utilities are trivially testable without React. Hook stays minimal and focused.

### Canonical query string as TanStack Query key

- Decision: Build one canonical query-string from normalized params, use it both for the fetch URL and as `['traces', canonicalQueryString]`.
- Why: Guarantees cache identity matches request identity. No stale cache on param reorder.

### Normalization rules for all URL params

- Decision: The parser normalizes every param on read. Specific rules:
  - `q`: trim whitespace; if result is empty, treat as unset (no chip, no param in URL). Prevents ghost chips when the backend ignores whitespace-only queries.
  - `status`: canonical URL form is **lowercase** (`running`, `completed`, `failed`). Accept case-insensitive input and normalize to lowercase. Canonicalize known backend aliases: `error` → `failed`. Truly unknown values (e.g., `pending`, `cancelled`) are dropped to unset.
  - `session_id`: validate as UUID format (`/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i`). Invalid values are dropped to unset rather than forwarded as a doomed request.
  - `min_duration_ms`: parse with `Number()`; drop if NaN, negative, 0, or non-integer (`!Number.isInteger()`). Non-integer values like `1.9` are treated as invalid/unset rather than silently truncated.
  - `offset`: parse as integer; drop if NaN or negative; reset to 0.
  - `has_errors`: only activates when the param is exactly `"true"` (case-sensitive).
  - `start_time_from`, `start_time_to`: must be valid ISO date-time strings; invalid values dropped.
- Why: Defensive normalization prevents malformed shared URLs from producing fetch errors or phantom UI state.

### `session_id` as URL-only supported filter

- Decision: `session_id` has no visible filter control in the filter bar. It is accepted from the URL (e.g., deep links from session pages), applied to the query, surfaced as an active chip, and clearable via chip dismiss or "Clear all."
- Why: Adding a session picker control adds complexity without clear user value in Phase 6. Deep links from the session detail page cover the primary use case.

### `min_duration_ms` input semantics

- Decision: Render as `<input type="number" min="1" step="1">`. Accept positive integers only. 0 is treated as unset (equivalent to no filter). Non-integer values (e.g., `1.9`) are treated as invalid/unset rather than silently truncated.
- Why: The backend parameter is `int64`. Allowing decimals or negative values creates a mismatch.

### Debounce text inputs, commit selects immediately

- Decision: `q`, `user_id`, `min_duration_ms` use 300ms debounce + Enter to commit. `status`, `has_errors`, dates commit on change.
- Why: Text inputs fire on every keystroke; debouncing avoids excessive requests. Selects and dates produce final values on change.
- Not committing on blur: prevents accidental partial-value commits when tabbing through controls.

### Native `<input type="date">` for date range

- Decision: Use native date inputs instead of a date picker library.
- Why: Reduces bundle size, avoids new dependencies, provides consistent mobile UX. Convert local dates to ISO boundary timestamps (start-of-day / end-of-day).
- Timezone note: Committed ISO timestamps are derived from the user's local timezone. A shared URL may render as a different calendar day in another timezone. This is acceptable for Phase 6; the alternative (UTC-only date pickers) hurts local usability more than cross-timezone sharing helps.

### History push for user actions, replace for normalization

- Decision: Filter/pagination changes push history entries. Canonicalization and stale-offset repair use `replace`.
- Why: Push preserves user's navigation trail. Replace avoids cluttering history with auto-corrections.

### `keepPreviousData` during refetch

- Decision: Use `placeholderData: keepPreviousData` in TanStack Query.
- Why: Prevents content flash when changing filters. Previous rows stay visible with an updating indicator.

### Chips rendered only when active

- Decision: Active-chip row is conditional on having at least one non-default filter param.
- Why: Chips are an explicit Phase 6 deliverable but should not consume space when unused.

## Risks / Trade-offs

- Native date inputs have inconsistent styling across browsers -> Acceptable for Phase 6; a date picker library can replace later.
- No blur-commit means a user could type a filter and navigate away without it applying -> Acceptable; avoiding accidental commits while tabbing away is the higher-priority behavior for Phase 6.
- `session_id` is accepted from URL but has no dedicated filter control -> Keeps the filter bar simple while supporting deep links from session pages.
- Shared date-filter URLs may show a different calendar day in other timezones -> Acceptable; local-date UX is preferred over UTC-only pickers.
- Return navigation depends on router state which is lost on refresh/new-tab -> Falls back to `/traces`; full durability would require encoding the return URL in the route itself, which is out of scope.

## Open Questions

None — all ambiguities resolved in the plan review.
