# Overview, Traces, And Sessions Pages

## Current state pattern

The operator console keeps overview shell navigation static while traces, sessions, session detail, and compare state stay URL-driven where it matters.

Use existing hooks and utilities instead of re-inventing parsing logic:
- `useTracesSearchParams`
- `useSessionsSearchParams`
- `tracesSearchParams.ts`
- `sessionsSearchParams.ts`

## Overview route

The `/` route is implemented and should stay frontend-only:
- it uses existing trace and session list endpoints
- it does not require new analytics APIs
- it acts as a snapshot/jump surface, not a chart-heavy dashboard

## Traces page

Implemented behavior includes:
- debounced text filters
- filter chips and clear actions
- sort state
- page size and pagination state
- back-link continuity into trace detail
- denser row rendering with trace name/status emphasized ahead of secondary metrics

## Sessions page

Implemented behavior includes:
- URL-driven search and user filters
- sort by created time or trace count
- pagination and stale-offset repair
- external-ID-first rendering
- row-level navigation back into session detail with `returnTo`

## Session detail page

The trace table under `/sessions/:id` also uses URL-driven table state:
- `sort_by`
- `sort_dir`
- `limit`
- `offset`

Keep trace-detail return navigation compatible with session-detail URLs.

Compare state on session detail is also URL-backed:
- `baseline_trace_id`
- `candidate_trace_id`

## Session compare page

`/sessions/:id/compare` is a first-class route.

Important current behavior:
- it reuses the compare query params from session detail
- it preserves `returnTo` navigation back into session detail
- it is sticky-header oriented and optimized for baseline/candidate scanability before row-level diff inspection
