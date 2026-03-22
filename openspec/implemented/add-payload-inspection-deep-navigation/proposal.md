# Phase 8: Payload Inspection & Deep Navigation

## Status: DRAFT

## Summary

Replace the static `<pre>` JSON renderer with an interactive payload inspector and add URL-driven span deep-linking, copy utilities, unified navigation callbacks, and truncation banners. All changes are frontend-only.

## Motivation

The current `JsonViewer` renders payloads as raw `JSON.stringify` output in a `<pre>` tag. This is unusable for large observability payloads (LLM prompts, tool responses) where users need to expand/collapse subtrees, search for keys, and copy values. Additionally, span selection is not URL-persisted, so users cannot share or bookmark links to specific spans.

Phase 6 established URL-driven filter state for the trace list. Phase 7 added failure-first auto-selection and breadcrumb navigation. Phase 8 extends both patterns: URL-driven span state on the trace detail page and an interactive inspector for every payload surface.

## Scope

- **In scope**: PayloadInspector component, span deep-linking via `?span=`, copy/clipboard utilities, unified span selection callback, truncation banners, per-inspector search.
- **Out of scope**: Backend changes, virtualized rendering, file export, OS share sheets, JSONPath query language, global cross-inspector search.

## Capabilities

| Capability | Spec | Description |
|------------|------|-------------|
| Payload Inspector | `payload-inspector/spec.md` | Interactive JSON tree with expand/collapse, search, copy, multiline rendering |
| Span Deep-Linking | `span-deep-linking/spec.md` | URL-driven selected span state with `?span=<external-span-id>` |
| Clipboard Utilities | `clipboard-utilities/spec.md` | Reusable CopyButton component and clipboard helper |
| Navigation Continuity | `navigation-continuity/spec.md` | Unified selection callback, parent span navigation button |
| Truncation Indicators | `truncation-indicators/spec.md` | Banners for truncated span payloads using existing API metadata |

## Delivery Order

1. **Payload Inspector** - Hardest part; unlocks every payload surface
2. **Span Deep-Linking** - High-value; isolated to trace detail state management
3. **Clipboard Utilities** - Simple shared infrastructure
4. **Navigation Continuity** - Refactors existing selection entry points through one path
5. **Truncation Indicators + Inspector Rollout** - Swap all payload surfaces once core is stable

## Design Decisions

See `design.md` for architectural reasoning on:
- Inspector rendering model and performance thresholds
- URL state interaction with Phase 7 auto-selection
- Search state scoping

## Files Affected

### New Files
- `web/src/components/PayloadInspector.tsx` - Interactive JSON tree
- `web/src/components/CopyButton.tsx` - Reusable copy control
- `web/src/utils/clipboard.ts` - Clipboard helper
- `web/src/utils/traceDetailSearchParams.ts` - URL param parse/serialize
- `web/src/hooks/useTraceDetailSearchParams.ts` - Hook wrapping useSearchParams

### Modified Files
- `web/src/pages/TraceDetailPage.tsx` - URL-driven span state, unified selection callback
- `web/src/components/SpanDetail.tsx` - PayloadInspector integration, truncation banners, parent navigation
- `web/src/components/Timeline.tsx` - PayloadInspector integration
- `web/src/components/JsonViewer.tsx` - Thin wrapper or removal

## Assumptions

- Scope stays frontend-only
- External `span_id` is the only deep-link identifier
- Search is per-inspector, not page-global
- Automatic failure-first selection remains local state; only explicit user-driven selection mutates the browser URL
- Existing Phase 6 back-link behavior and Phase 7 failure-first behavior remain unchanged
- React Query fetch keys remain trace-id based; changing `?span=` does not trigger refetches
