# Trace Workspace

## Main coordinator

`web/src/pages/TraceDetailPage.tsx` owns:
- trace and span queries
- running timeline polling
- selection and reveal behavior
- return navigation
- desktop vs mobile workspace composition
- trace-context drawer state

## Desktop layout

`WorkspaceShell` renders the main investigation panes:
- left: `TreeRail`
- top-right: `ExecutionWaterfall`
- bottom-right: `InspectorTabs`

Desktop trace context is no longer a permanently visible panel in that grid. It is opened separately as a right-side drawer from trace detail.

`InspectorTabs` renders:
- `Details`
- `Timeline`
- `State`

The tabs stay mounted while hidden so local UI state survives tab switches.

## Mobile layout

Top-level mobile tabs are:
- `Summary`
- `Execution`
- `Timeline`
- `State`

Inside `Execution`, mobile keeps a local `Waterfall` / `Tree` sub-toggle.

## Shared selection model

Selection should stay aligned across:
- tree row click
- waterfall bar click
- timeline jump
- breadcrumb navigation
- failure-summary jump actions
- parent-span navigation

Current selection state is coordinated through:
- `useWorkspaceState`
- `useTraceDetailSearchParams`
- `buildBreadcrumbPath`
- `serializeSpanParam`

## Data surfaces already implemented

- failure-first summary
- running-state / wait-stall summary
- stale-trace signal
- payload inspection with truncation banners
- decision rendering in span detail
- state diff extracted from `state_change` timeline events
- semantic timeline summaries for `state_change` and `decision`
- local quick filters in `TreeRail` using already loaded span data only

## Test anchors

When changing this area, the most useful frontend coverage usually lives in:
- `web/src/pages/TraceDetailPage.test.tsx`
- `web/src/components/AppShell.test.tsx`
- `web/src/components/InspectorTabs.test.tsx`
- `web/src/components/PayloadInspector.test.tsx`
- `web/src/components/StateDiffViewer.test.tsx`
- `web/src/utils/failureAnalysis.test.ts`
- `web/src/utils/spanTree.test.ts`
- `web/src/utils/waterfallTime.test.ts`
