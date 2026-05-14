# Change: Add trace-level reasoning tab and cost surfaces to debugger

## Why
The debugger workspace shows per-span decisions and metrics, but there is no trace-level view of the reasoning chain or cumulative cost shape. Operators debugging multi-span agent traces need a single chronological reasoning log and a visual cost curve without clicking through individual spans.

## What Changes
- Add a **Reasoning** tab to the desktop inspector and mobile workspace that lists all valid `decision` events across the trace in chronological order, with click-to-navigate to the originating span.
- Add a **cumulative cost strip** as a fixed row between the waterfall time-axis header and the virtualized row scroller, rendered as an inline SVG step chart aligned to the existing time axis.
- Add **per-row cost/token annotations** in the waterfall label column for spans with non-zero cost or token data.
- All new behavior is derived client-side from existing `Span`, `TraceDetail`, and `TimelineEvent` data. No backend, API, migration, or storage changes.

## Impact
- Affected specs: new `debugger-reasoning`, new `debugger-cost-surfaces`
- Affected code:
  - `web/src/pages/TraceDetailPage.tsx` — derive reasoning entries and cost series, wire new tab
  - `web/src/components/InspectorTabs.tsx` — add `reasoning` tab id and content slot
  - `web/src/components/WorkspaceShell.tsx` — add `reasoning` mobile tab
  - `web/src/components/ExecutionWaterfall.tsx` — cost strip and per-row annotations
  - `web/src/components/ReasoningTab.tsx` — new component (trace-level decision list)
  - `web/src/components/CostStrip.tsx` — new component (SVG step chart)
  - `web/src/utils/reasoning.ts` — decision extraction and cost series derivation utilities
  - `web/src/utils/reasoning.test.ts` — unit tests for the above
