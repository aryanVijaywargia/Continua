## 1. Failure Analysis Helper

- [x] 1.1 Create `web/src/utils/failureAnalysis.ts` with pure functions: `buildSpanIndex`, `findPrimaryFailedSpan`, `buildBreadcrumbPath`, `getInlineErrorPreview`, `buildFailureSummary`, `evaluateStaleTraceSignal`
- [x] 1.2 Implement cycle-safe parent chain traversal with visited-set guard
- [x] 1.3 Implement primary failed span selection with deterministic tie-breaking (ended_at asc → started_at asc → array order)
- [x] 1.4 Implement inline error preview extraction (span.error_message → first error/exception event message → null; trim + first line + 120 char truncation). Note: intentionally narrower than the error-only filter predicate — excludes `span_failed` and `level === 'error'` events
- [x] 1.5 Implement error event count using the shared `isTimelineErrorEvent` predicate (consistent with the error-only timeline filter)
- [x] 1.6 Implement stale trace signal heuristic (RUNNING + 15min runtime + 5min since last activity)
- [x] 1.7 Add unit tests for all helper functions in `web/src/utils/failureAnalysis.test.ts`

**Validation**: `pnpm --filter web test -- failureAnalysis`

## 2. Selection State, Auto-Selection, and Data Refresh

- [x] 2.1 Refactor `TraceDetailPage.tsx` selection state to use `selectedSpanExternalId: string | null` + `userHasSelected: boolean`
- [x] 2.2 Build memoized `spanByExternalId` map from spans array
- [x] 2.3 Implement auto-selection logic: select primary failed span on initial failed load or running→failed transition, only when `!userHasSelected`
- [x] 2.4 Implement sticky selection: set `userHasSelected = true` on manual span selection
- [x] 2.5 Implement fallback: if selected span disappears from refreshed data, fall back to primary failed span or clear
- [x] 2.6 Add terminal refresh: when timeline hook detects terminal transition (RUNNING → COMPLETED/FAILED), invalidate `traceQuery` and `spansQuery` via `queryClient.invalidateQueries` so failure analysis, header metrics, and summary operate on current data
- [x] 2.7 Add cross-trace state reset: use `key={traceId}` on `TraceDetailContent` (or equivalent) to reset all Phase 7 local state when navigating between traces
- [x] 2.8 Add integration tests for:
  - Auto-selection on initial failed load
  - Sticky selection across polling/re-render
  - Running trace transitions to failed → refetched spans drive auto-selection
  - Navigate from trace A to trace B → selection/filter state is clean

**Validation**: `pnpm --filter web test -- TraceDetailPage`

## 3. Failure Summary Component

- [x] 3.1 Create `web/src/components/FailureSummary.tsx` rendering above Trace Context when effective trace status is `FAILED`
- [x] 3.2 Display: primary failed span name/kind, error preview, failure timestamp, failed-span count, error-event count, breadcrumb path, jump action
- [x] 3.3 Handle edge case: failed trace with no failed spans shows generic message, omits jump action
- [x] 3.4 Ensure jump action and all interactive elements are keyboard accessible with explicit accessible names
- [x] 3.5 Add integration tests for failure summary rendering and jump action

**Validation**: `pnpm --filter web test`

## 4. Span Tree Highlighting

- [x] 4.1 Add `failedSpanIds: Set<string>` and `primaryAncestorPath: Set<string>` props to SpanTree/SpanTreeNode
- [x] 4.2 Highlight all failed rows with visual indicator (not color-only: include text label or badge)
- [x] 4.3 Highlight primary ancestor branch distinctly
- [x] 4.4 Implement style precedence: selected > ancestor path > failed row
- [x] 4.5 Add `revealPath: Set<string>` prop; nodes on the path auto-expand if collapsed (applies to auto-selection, jump action, breadcrumb clicks, and timeline clicks)
- [x] 4.6 Add inline error previews on failed rows using failure analysis helper
- [x] 4.7 Add integration tests for highlighting, reveal behavior, and style precedence

**Validation**: `pnpm --filter web test`

## 5. Span Breadcrumb Component

- [x] 5.1 Create `web/src/components/SpanBreadcrumb.tsx` showing root-to-selected span path
- [x] 5.2 Make ancestor segments clickable (triggers span selection via shared callback — full tree/detail/timeline synchronization)
- [x] 5.3 Ensure keyboard accessibility for all breadcrumb segments
- [x] 5.4 Integrate into SpanDetail panel header
- [x] 5.5 Add integration tests for breadcrumb rendering, click behavior, and full synchronization

**Validation**: `pnpm --filter web test`

## 6. Error-Only Timeline Filter

- [x] 6.1 Add `Errors only` toggle button in Timeline header with `aria-pressed`
- [x] 6.2 Filter events client-side using `isTimelineErrorEvent` predicate
- [x] 6.3 Show filtered empty state when active and no matching events
- [x] 6.4 Ensure toggle is keyboard accessible
- [x] 6.5 Ensure filter state resets on cross-trace navigation (handled by key-based remounting)
- [x] 6.6 Add integration tests for filter toggle behavior

**Validation**: `pnpm --filter web test`

## 7. Stale Trace Signal

- [x] 7.1 Add stale trace signal rendering in TraceDetailPage for qualifying traces
- [x] 7.2 Use local constants for thresholds (15min runtime, 5min stale)
- [x] 7.3 Derive latest activity from: max event timestamp → latest span ended_at → latest span started_at → trace started_at
- [x] 7.4 Keep copy subtle and non-authoritative; mark as experimental
- [x] 7.5 Ensure signal does not re-announce on polling (informational text, not alert — no `role="alert"` or `aria-live`)
- [x] 7.6 Add unit test coverage for threshold logic
- [x] 7.7 Add rendered integration test asserting signal appears as static text (not alert/live region) in the DOM

**Validation**: `pnpm --filter web test`

## 8. Final Verification

- [x] 8.1 Run full test suite: `pnpm --filter web test`
- [x] 8.2 Run type checking: `pnpm --filter web type-check`
- [x] 8.3 Verify Phase 6 filtered back-link behavior is preserved
- [ ] 8.4 Manual accessibility check: keyboard navigation through all new controls
