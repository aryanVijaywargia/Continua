# Traces And Sessions Pages

## Current state pattern

The list pages are URL-driven.

Use existing hooks and utilities instead of re-inventing parsing logic:
- `useTracesSearchParams`
- `useSessionsSearchParams`
- `tracesSearchParams.ts`
- `sessionsSearchParams.ts`

## Traces page

Implemented behavior includes:
- debounced text filters
- filter chips and clear actions
- sort state
- page size and pagination state
- back-link continuity into trace detail

## Sessions page

Implemented behavior includes:
- URL-driven search and user filters
- sort by created time or trace count
- pagination and stale-offset repair
- external-ID-first rendering

## Session detail page

The trace table under `/sessions/:id` also uses URL-driven table state:
- `sort_by`
- `sort_dir`
- `limit`
- `offset`

Keep trace-detail return navigation compatible with session-detail URLs.
