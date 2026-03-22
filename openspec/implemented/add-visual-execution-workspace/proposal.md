# Change: Add Visual Execution Workspace

## Why
The current trace detail page stacks the span tree, span detail, and timeline vertically, making it hard to correlate execution timing with span hierarchy on desktop screens. Debugging requires constant scrolling between panels. A split-panel workspace with an execution waterfall provides a professional debugger experience that shows timing and hierarchy simultaneously.

## What Changes
- Refactor `TraceDetailPage` into a modular workspace shell with three regions: tree rail (left), waterfall (top-right), and inspector tabs (bottom-right)
- Add `react-resizable-panels` for desktop split-panel mechanics; non-desktop uses a 4-tab stacked layout (`Waterfall`, `Tree`, `Details`, `Timeline`)
- Extract tree-building, preorder flattening, and expansion logic into shared pure helpers
- Replace per-node local expansion state with shared `expandedSpanIds` managed at workspace level
- Add an execution waterfall that mirrors the visible tree preorder row model
- Add lightweight windowed rendering for the tree rail and waterfall so large traces stay within the Phase 9 interaction budgets without changing the logical visible-row model
- Add inspector tabs (`Details` + `Timeline`) with mounted-while-hidden retention
- Move Failure Summary and Stale Trace Signal into the `Details` inspector tab
- Collapse Trace Context by default (disclosure on desktop, inside `Details` tab on non-desktop)
- Add tree search (non-destructive, auto-expand matches, dim non-matches) and tree controls (`Expand all`, `Collapse all`, `Show metrics` toggle)
- Keep `?span=<external-span-id>` as the only URL-persisted state

## Impact
- Affected specs: `workspace-layout`, `execution-waterfall`, `tree-search-controls`, `inspector-tabs`, `shared-workspace-state`
- Affected code: `web/src/pages/TraceDetailPage.tsx`, `web/src/components/SpanTree.tsx`, `web/src/components/SpanDetail.tsx`, `web/src/components/Timeline.tsx`, `web/src/components/FailureSummary.tsx`
- New files: `web/src/pages/TraceDetailPage.test.tsx` (tests moved from `TracesPage.test.tsx`), `web/src/utils/spanTree.ts`, `web/src/utils/waterfallTime.ts`, `web/src/components/WorkspaceShell.tsx`, `web/src/components/InspectorTabs.tsx`, `web/src/components/ExecutionWaterfall.tsx`, `web/src/hooks/useWorkspaceState.ts`, `web/src/hooks/useVirtualRows.ts`
- New dependency: `react-resizable-panels`
- Frontend-only change; no backend or API modifications
