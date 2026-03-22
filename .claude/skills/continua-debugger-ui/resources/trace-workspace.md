# Trace Workspace

## Main coordinator

`web/src/pages/TraceDetailPage.tsx` owns:
- trace and span queries
- running timeline polling
- selection and reveal behavior
- return navigation
- desktop vs mobile workspace composition

## Desktop layout

`WorkspaceShell` renders:
- left: `TreeRail`
- top-right: `ExecutionWaterfall`
- bottom-right: `InspectorTabs`

`InspectorTabs` renders:
- `Details`
- `Timeline`
- `State`

The tabs stay mounted while hidden so local UI state survives tab switches.

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
- stale-trace signal
- payload inspection with truncation banners
- decision rendering in span detail
- state diff extracted from `state_change` timeline events
- semantic timeline summaries for `state_change` and `decision`

## Test anchors

When changing this area, the most useful frontend coverage usually lives in:
- `web/src/pages/TraceDetailPage.test.tsx`
- `web/src/components/InspectorTabs.test.tsx`
- `web/src/components/PayloadInspector.test.tsx`
- `web/src/components/StateDiffViewer.test.tsx`
- `web/src/utils/failureAnalysis.test.ts`
- `web/src/utils/spanTree.test.ts`
- `web/src/utils/waterfallTime.test.ts`
