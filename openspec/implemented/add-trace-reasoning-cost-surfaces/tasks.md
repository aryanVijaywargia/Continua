## 1. Reasoning utilities and tests
- [x] 1.1 Create `web/src/utils/reasoning.ts` with `buildReasoningEntries(events, spans)` that filters valid decisions via `getDecisionDetails()`, resolves span name from `event.span_name` first (falling back to `spans` lookup only when absent), and sorts using `compareTimelineEvents()` from `timeline.ts`. Export `DecisionTraceEntry` type.
- [x] 1.2 Create `buildTraceCostSeries(spans, traceStatus)` in the same file that derives `TraceCostPoint[]` and cumulative total from terminal span `cost_usd`, anchored at `ended_at` (fallback `started_at`), excluding running spans. Collapse tied anchor timestamps into single combined steps at the utility level. Return `null` (not an empty array) for zero-cost traces. Mark the series as `partial: true` when `traceStatus` is `RUNNING`. Export `TraceCostPoint` and `TraceCostSeries` types.
- [x] 1.3 Add unit tests in `web/src/utils/reasoning.test.ts` covering: valid vs malformed decisions, tied timestamps (verify `compareTimelineEvents` ordering), cross-span ordering, missing span names (fallback path), completed traces, running traces with partial totals and `partial: true`, zero-cost traces (returns `null`), mixed costed/non-costed spans, tied anchor timestamps (verify aggregation), terminal spans missing `ended_at`.

## 2. Reasoning tab component
- [x] 2.1 Create `web/src/components/ReasoningTab.tsx` — chronological list of `DecisionTraceEntry` rows. Each row is a `<button>` element for keyboard accessibility. Shows timestamp, span name, question, chosen, optional reasoning/alternatives. Accept `onSelectSpan` callback. Render empty state when no entries.
- [x] 2.2 Add component test for ReasoningTab: renders entries, renders empty state, click fires `onSelectSpan`, keyboard Enter/Space fires `onSelectSpan`.

## 3. Wire Reasoning tab into workspace
- [x] 3.1 Extend `InspectorTabId` in `InspectorTabs.tsx` to include `'reasoning'`. Add the tab button and content slot.
- [x] 3.2 Extend `MobileWorkspaceTabId` in `WorkspaceShell.tsx` to include `'reasoning'`. Add the tab button and content projection. Acceptance: six tabs must remain reachable without horizontal overflow scrolling on 320px-wide viewports; wrapping is allowed.
- [x] 3.3 In `TraceDetailPage.tsx`, add `useMemo` derivation for `DecisionTraceEntry[]` from `timeline.events` and `spans`. Wire into ReasoningTab with navigation: desktop uses `switchToDetailsRef`, mobile sets active tab to `details`.
- [x] 3.4 Add integration test: clicking a Reasoning row selects span and switches to Details.

## 4. Cost strip component
- [x] 4.1 Create `web/src/components/CostStrip.tsx` — inline SVG step chart accepting pre-aggregated `TraceCostSeries` and waterfall `window`. Left cell: "Cumulative cost" label. Right cell: step line with endpoint marker and total label, aligned to waterfall time axis. When `series.partial` is true, append a "Partial" indicator next to the total label. The component does not perform timestamp collapsing; that is owned by `buildTraceCostSeries()`. When `series` is `null`, render nothing (no DOM output).
- [x] 4.2 Add component test: renders step chart for cost series, renders "Partial" indicator for running traces, hidden entirely (no DOM output) when series is `null`.

## 5. Waterfall integration
- [x] 5.1 Insert `CostStrip` into `ExecutionWaterfall.tsx` between the time-axis header and the virtualized scroller. Pass derived cost series and waterfall window.
- [x] 5.2 Add compact inline token/cost annotations to the existing status/duration line in the waterfall label column for rows with non-zero token or cost data. Use existing `formatTokens` and `formatCost` utilities. Annotations MUST fit within the existing uniform `WATERFALL_ROW_HEIGHT` (68px); no variable-height rows are introduced.
- [x] 5.3 In `TraceDetailPage.tsx`, add `useMemo` derivation for `TraceCostSeries` from `spans` and trace status. Pass to waterfall.
- [x] 5.4 Add integration test for running-trace cost update: mock a span query result update (simulating existing polling refresh), assert the cost strip re-derives with updated totals and shows partial indicator, and assert no new polling path is introduced.
- [x] 5.5 Add waterfall label column test: verify annotation text appears for cost-bearing rows, does not appear for zero-cost rows, and row height remains uniform.

## 6. Verify
- [x] 6.1 Run `pnpm --filter web test` and confirm all new and existing tests pass.
- [x] 6.2 Manual QA (not jsdom geometry tests): SVG strip alignment against time axis, desktop resize behavior, waterfall scroll/virtualization interaction, mobile tab layout (six tabs reachable without clipping on small screens). Verified with Playwright browser QA at 1280px desktop, 1100px desktop resize, and 320px mobile; screenshots captured in `/tmp/continua-trace-reasoning-desktop.png` and `/tmp/continua-trace-reasoning-mobile.png`.
- [x] 6.3 Run existing workspace profiling script if the new strip measurably affects render cost. Not required: final fixes are tab routing and layout alignment/responsiveness changes, with no measured render-cost-sensitive change beyond the existing waterfall virtualization path.
