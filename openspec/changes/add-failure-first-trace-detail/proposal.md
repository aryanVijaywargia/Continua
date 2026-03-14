# Change: Add Failure-First Trace Detail Experience

## Why

When debugging failed AI agent traces, users currently must manually scan the span tree and timeline to locate failures. There is no automated guidance toward the root cause. This makes triage slow and error-prone, especially for deep span trees with many nodes.

Phase 7 delivers a failure-first experience: the page automatically surfaces the primary failed span, highlights the failure path, and provides contextual navigation so users can understand what went wrong without hunting through the tree.

## What Changes

- **Failure analysis helper** — Pure TypeScript module that computes primary failed span, failure summary, inline error previews, breadcrumb paths, and a stale-trace trust signal. All logic is memoized and cycle-safe.
- **Failure summary UI** — New `FailureSummary` component rendered above Trace Context for failed traces. Shows failure details and a jump-to action.
- **Auto-selection** — Page auto-selects the primary failed span on initial load or when a running trace transitions to failed, but respects sticky user selection. Terminal transition triggers a refetch of trace and spans data so failure analysis operates on current state.
- **Cross-trace state reset** — All Phase 7 local state (selection, user-has-selected flag, error filter, reveal path) resets when navigating between traces to prevent stale state leakage.
- **Span tree highlighting** — Failed span rows are visually marked; the ancestor branch of the primary failed span is highlighted distinctly.
- **Span breadcrumb** — New `SpanBreadcrumb` component at the top of the detail panel showing root-to-selected path with clickable ancestors.
- **Error-only timeline filter** — Toggle button in the timeline header to filter to error events only.
- **Stale trace signal** — Subtle informational text for long-running traces with sparse recent activity.

All changes are frontend-only. No OpenAPI, Go, SQL, migration, or codegen changes.

## Impact

- Affected specs: (new) `failure-analysis`, `failure-summary-ui`, `span-breadcrumb`, `error-timeline-filter`, `stale-trace-signal`
- Affected code:
  - `web/src/pages/TraceDetailPage.tsx` — selection state, layout, new components
  - `web/src/components/SpanTree.tsx` — failure highlighting, reveal-path signal
  - `web/src/components/SpanDetail.tsx` — breadcrumb integration
  - `web/src/components/Timeline.tsx` — error-only filter toggle
  - New: `web/src/utils/failureAnalysis.ts` — pure helper module
  - New: `web/src/components/FailureSummary.tsx`
  - New: `web/src/components/SpanBreadcrumb.tsx`
- Dependencies: Phase 6 (trace discovery/triage) must be merged or rebased first
- Breaking changes: None
