## Context

Phase 7 adds failure-first UX to the trace detail page. This is a coordination/state-management phase that builds on the existing span tree, timeline, and detail panel without changing the data model or API contracts.

The primary challenge is managing selection state correctly across polling, auto-selection, and user interaction while keeping derived failure computations efficient and cycle-safe.

## Goals / Non-Goals

- Goals:
  - Deterministic primary failed span selection with stable tie-breaking
  - Sticky user selection that survives polling and recomputation
  - Linear, memoized failure analysis computed once per data change
  - Cycle-safe parent chain traversal for breadcrumbs and highlighting
  - Accessible keyboard-operable controls for all new interactions
- Non-Goals:
  - Changing the data model or API contracts
  - Adding error boundaries (app has no existing pattern)
  - Lifting full tree expansion state to the page level
  - Server-side failure analysis or ranking

## Decisions

### Selection state model

- **Decision**: Use `selectedSpanExternalId: string | null` as the canonical page-level key, with a `userHasSelected: boolean` flag to distinguish auto-selection from user selection.
- **Why**: External `span_id` is the key used across tree ancestry, timeline span references, and failure-path logic. The `userHasSelected` flag prevents polling from overriding manual choices.
- **Alternatives**: Using internal `id` was considered but rejected because parent-child relationships and timeline events reference external `span_id`.

### Failure analysis as a pure helper

- **Decision**: One pure TypeScript module (`failureAnalysis.ts`) with no React dependencies. Consumers call it with spans/events and get derived state back.
- **Why**: Keeps logic testable without rendering, allows memoization at the call site via `useMemo`, and avoids coupling analysis logic to component lifecycle.
- **Alternatives**: Custom hook was considered but a pure module is simpler and more portable.

### Primary failed span determinism

- **Decision**: Sort failed spans by `ended_at` ascending (earliest failure first), then `started_at`, then original array order. Take the first.
- **Why**: The earliest failure is most likely the root cause. Deterministic tie-breaking prevents selection flickering during polling.

### Tree highlighting without lifting expansion state

- **Decision**: Pass a `revealPath: Set<string>` (set of external span IDs) from the page. Each `SpanTreeNode` checks if it's on the path and auto-expands if collapsed. Expansion state remains local to nodes.
- **Why**: Avoids the complexity of controlled expansion for the entire tree. The reveal-path is a one-way signal that nodes can consume independently.

### Error-only filter placement

- **Decision**: Keep the toggle inside `Timeline.tsx`, driven by local state. The filter uses the existing `isTimelineErrorEvent` predicate.
- **Why**: The filter is self-contained within the timeline and doesn't affect other components. No need to lift state.

### Terminal refresh of trace and spans

- **Decision**: When the timeline hook detects a terminal transition (`RUNNING` → `COMPLETED`/`FAILED`), invalidate the `traceQuery` and `spansQuery` via TanStack Query's `queryClient.invalidateQueries`. This triggers a refetch so failure analysis, header metrics, and the summary operate on current data.
- **Why**: Currently `traceQuery` and `spansQuery` are one-shot fetches. Without invalidation after terminal transition, the failure-first UI would be driven by stale data (e.g., missing failed spans that arrived after initial load).
- **Alternatives**: Adding independent polling for trace/spans was considered but rejected — the timeline hook already detects terminal state, so piggyback on that signal.

### Cross-trace state reset

- **Decision**: Use a `key={traceId}` prop on `TraceDetailContent` (or equivalent reset via `useEffect` on `traceId`) to reset all Phase 7 local state when the route parameter changes.
- **Why**: The `/traces/:id` route reuses the same component, so React won't unmount/remount by default. Without explicit reset, `selectedSpanExternalId`, `userHasSelected`, error filter state, and reveal-path state would leak between traces.
- **Alternatives**: Manual `useEffect` reset was considered but `key`-based remounting is simpler and guarantees all state (including state in child components like the timeline filter) resets.

## Risks / Trade-offs

- **Polling + auto-selection race**: If spans data and timeline data update at different times, the primary failed span could briefly differ. Mitigated by triggering trace/spans refetch on terminal transition, and by making user selection sticky.
- **Cycle-safe traversal overhead**: The visited-set check adds minimal overhead per traversal. For traces with thousands of spans this is negligible compared to rendering.
- **Stale trace signal false positives**: The 15-minute/5-minute thresholds are conservative defaults. Marked as experimental with local constants for easy tuning.
- **Terminal refetch brief loading state**: After invalidation, TanStack Query may briefly show stale data while refetching. This is acceptable — the stale-while-revalidate pattern means the UI stays interactive.

## Open Questions

- None blocking implementation. Threshold tuning for the trust signal can be adjusted post-launch based on real usage data.
