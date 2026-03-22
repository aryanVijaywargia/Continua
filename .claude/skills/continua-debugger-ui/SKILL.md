---
name: continua-debugger-ui
description: Guide for Continua's current debugger frontend. Use when changing trace detail workspace behavior, traces or sessions pages, payload inspection, state diff, settings, command palette, theming, or other debugger UX in web/src.
---

# Continua Debugger UI

## Read first
- [../references/decisions.md](../references/decisions.md)
- [../../../docs/DEBUGGER_PLATFORM_BASELINE.md](../../../docs/DEBUGGER_PLATFORM_BASELINE.md)

## Use this skill when
- editing `web/src/pages`, `web/src/components`, `web/src/hooks`, or `web/src/utils`
- changing trace detail workspace layout or selection behavior
- changing traces or sessions list URL state, sorting, filters, or pagination
- changing payload inspection, truncation banners, state diff, or timeline rendering
- changing settings, auth recovery, command palette, or theme behavior

## Current debugger shape
- `App.tsx` routes to `/traces`, `/traces/:id`, `/sessions`, `/sessions/:id`, and `/settings`
- `TraceDetailPage.tsx` is the main debugger workspace coordinator
- desktop trace detail uses `WorkspaceShell` with `TreeRail`, `ExecutionWaterfall`, and `InspectorTabs`
- mobile trace detail keeps the same surfaces behind tab switches
- `InspectorTabs` keeps `details`, `timeline`, and `state` mounted while hidden
- `SpanDetail` owns the detailed selected-span surface; `Timeline` and waterfall selection should route back into the same shared span-selection path

## State and routing rules
- Preserve URL-driven state for list pages and trace detail.
- `/traces` and `/sessions` state belongs in search params, not local-only `useState`.
- selected span state belongs in `?span=` via `useTraceDetailSearchParams`
- do not include UI-only URL params in React Query keys unless the server request depends on them
- keep back-link continuity working from traces or sessions into trace detail and back

## Implementation guardrails
- keep complex view-model logic in pure utilities or small hooks:
  - `useWorkspaceState`
  - `useTraceDetailSearchParams`
  - `useTracesSearchParams`
  - `useSessionsSearchParams`
  - `spanTree.ts`
  - `waterfallTime.ts`
  - `failureAnalysis.ts`
  - `stateChanges.ts`
- keep tree, waterfall, breadcrumb, and timeline selection synchronized through shared callbacks
- preserve polling-based running-trace behavior; do not design around a live WebSocket runtime that does not exist
- reuse `PayloadInspector`, `TruncationBanner`, `CopyButton`, and `AuthErrorBanner` instead of creating duplicate patterns

## Useful references
- [trace-workspace.md](resources/trace-workspace.md)
- [list-pages.md](resources/list-pages.md)
