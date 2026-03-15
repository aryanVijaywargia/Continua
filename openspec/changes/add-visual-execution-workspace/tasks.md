## PR 1: Baseline extraction (no user-visible change)

- [x] 1.1 Move all `describe('TraceDetailPage')` integration tests from `TracesPage.test.tsx` into a dedicated `TraceDetailPage.test.tsx` with no logic changes
- [x] 1.2 Extract pure tree helpers: `buildSpanTree`, `flattenPreorder`, `getAncestorIds`, `deriveVisibleRows` into `web/src/utils/spanTree.ts`
- [x] 1.3 Extract shared workspace state hook (`useWorkspaceState`) managing `expandedSpanIds`, `selectedSpanExternalId`, `revealPath`, `revealVersion` (monotonically incrementing counter), `userHasSelected`, and waterfall reveal targeting; preserve the existing sticky-selection and versioned-reveal semantics from `TraceDetailPage.tsx`
- [x] 1.4 Initialize `expandedSpanIds` with all span IDs that have children so the tree renders the same fully-expanded default
- [x] 1.5 Refactor `SpanTree` to consume shared `expandedSpanIds` and `onToggleExpand` instead of per-node local state
- [x] 1.6 Add unit tests for tree building, preorder flattening, orphan/cycle handling, and shared expansion behavior
- [x] 1.7 Verify all existing trace-detail integration tests pass unchanged in `TraceDetailPage.test.tsx`

## PR 2: Desktop panel shell

- [x] 2.1 Install `react-resizable-panels`
- [x] 2.2 Create `WorkspaceShell` component with desktop split-panel layout: left tree rail, right panel with top/bottom split
- [x] 2.3 Create non-desktop stacked fallback rendering the same underlying content
- [ ] 2.4 Wire `TraceDetailPage` to render `WorkspaceShell` with tree rail (left), existing `SpanDetail` (right top), existing `Timeline` (right bottom)
- [x] 2.5 Add integration tests for desktop layout rendering tree and detail panels side by side
- [x] 2.6 Add integration tests for non-desktop stacked fallback

## PR 3: Inspector tabs (bottom-right only; SpanDetail stays in top-right)

- [x] 3.1 Create `InspectorTabs` component with `Details` (default) and `Timeline` tabs
- [x] 3.2 Replace the bottom-right `Timeline` panel with `InspectorTabs`; the `Timeline` tab contains the existing Timeline content
- [ ] 3.3 The `Details` tab contains Failure Summary, Stale Trace Signal, and a compact span summary (name, status, kind, breadcrumb) when a span is selected. When no span is selected, show the existing "Select a span to view details" empty state. Full SpanDetail content (payloads, identifiers, timestamps) remains in the top-right panel until PR 4 moves it here.
- [x] 3.4 Keep both tabs mounted (CSS `display: none` for hidden) so timeline-local state survives tab switches
- [x] 3.5 Expose `switchToDetails` callback from `InspectorTabs` for external panels to invoke
- [x] 3.6 Add integration tests for tab switching within the same trace
- [x] 3.7 Add integration tests for timeline state preservation across tab switches
- [x] 3.8 Add integration tests for Failure Summary and Stale Trace Signal rendering in Details tab

## PR 4: Execution waterfall

- [x] 4.1 Create `ExecutionWaterfall` component consuming the shared visible-row model
- [x] 4.2 Extract waterfall time helpers into `web/src/utils/waterfallTime.ts`: time-scale derivation from trace start/end times, span timestamps, and timeline snapshot (handling running traces where `ended_at` is absent by using the latest span/event timestamp as the right boundary)
- [x] 4.3 Implement sticky time axis header
- [x] 4.4 Implement keyboard-focusable timing bars with Enter/Space selection
- [x] 4.5 Implement running-span deterministic rendering (bar extends to trace boundary)
- [x] 4.6 Implement minimum visible width for very short spans
- [x] 4.7 Implement minimal tooltip (span name, status, duration)
- [x] 4.8 Wire waterfall bar selection to unified selection path + `switchToDetails`
- [x] 4.9 Replace right-top `SpanDetail` panel with `ExecutionWaterfall` in `WorkspaceShell` and move full SpanDetail content (payloads, identifiers, timestamps, parent navigation) into the `Details` tab
- [x] 4.10 Add unit tests for waterfall window calculation, zero-duration spans, and running-span handling
- [x] 4.11 Add integration tests for waterfall selection syncing tree, details, timeline, breadcrumb, and `?span=`
- [x] 4.12 Add integration tests for collapse/expand in tree changing waterfall visible rows
- [x] 4.13 Add windowed waterfall row rendering that preserves logical row order, reveal behavior, and keyboard selection on large traces

## PR 5: Tree search, metrics, Trace Context, polish

- [x] 5.1 Add tree search input with deferred search model
- [x] 5.2 Implement case-insensitive matching on span name, kind, span ID, status, model, provider, inline error preview
- [x] 5.3 Implement match highlighting and non-match dimming
- [x] 5.4 Implement auto-expand matched ancestors while search is active
- [x] 5.5 Implement prior expansion state save/restore when search clears
- [x] 5.6 Add `Expand All` button with projected visible-row expansion cost guard
- [x] 5.7 Add `Collapse All` button
- [x] 5.8 Add `Show Metrics` toggle for inline token count and cost hints on tree rows (duration and status badge remain always visible)
- [x] 5.9 Implement collapsed Trace Context: disclosure above workspace on desktop, section inside Details tab on non-desktop
- [ ] 5.10 (Optional) Add tree connector lines for readability polish
- [x] 5.11 Add unit tests for search-state restore behavior
- [x] 5.12 Add integration tests for search auto-revealing matches and restoring prior expansion
- [x] 5.12a Add windowed tree-row rendering that preserves logical visible-row semantics and reveal behavior on large traces
- [x] 5.13 Benchmark at 200, 400, 800, 1200 spans; validate expand/collapse stays within one frame for <500 spans, waterfall render <100ms, search doesn't block input beyond deferred cycle
- [x] 5.14 Finalize `Expand All` guard constant based on benchmark results
- [x] 5.15 Verify all existing back-link and failure-first behavior remains intact
