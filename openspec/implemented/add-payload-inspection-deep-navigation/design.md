# Design: Payload Inspection & Deep Navigation

## 1. Inspector Rendering Model

### Decision: Memoized tree with lazy child rendering

Build an in-memory tree of typed nodes (`ObjectNode`, `ArrayNode`, `PrimitiveNode`) once per payload change. Render children only when a node is expanded. Never call `JSON.stringify` on the full payload during normal rendering.

**Why not virtualized lists?** Observability payloads are typically wide (many keys) rather than deep (thousands of siblings). The initial collapse rules (depth >= 2 collapsed, shallow-size > 200 collapsed) bound the rendered node count well within React's capacity. Virtualization adds significant complexity for scroll-position management inside nested trees and is deferred to a future phase if profiling shows need.

### Collapse Thresholds

Two rules cooperate:

| Rule | Condition | Initial State |
|------|-----------|---------------|
| Depth rule | `depth >= 2` | Collapsed |
| Shallow-size rule | `> 200 immediate children` at depth 0 or 1 | Collapsed with count badge |

The shallow-size rule exists because depth-only rules still eagerly render large top-level arrays — common in batch LLM responses. The 200-child threshold is chosen to keep initial DOM node count below ~1000 for any single inspector instance.

### Expand All Gating

`Expand all` is disabled when total tree node count exceeds 5,000. This is a hard gate, not a warning. The count is computed once during tree construction and cached.

## 2. URL State and Phase 7 Interaction

### Decision: `?span=<external-span-id>` with `replace` semantics

Following Phase 6's URL-driven filter pattern, the selected span is persisted in the URL as `?span=<external-span-id>`.

**Key interaction with Phase 7 auto-selection:**

```
Page load with ?span=abc
  → Treat as explicit user intent
  → Set userHasSelected = true
  → Skip failure-first auto-selection

Page load without ?span=
  → Phase 7 auto-selects primary failed span (if applicable)
  → userHasSelected remains false
  → URL is NOT back-written (no ?span= appears)
  → But "Copy Trace URL" DOES include the effective span

User clicks a span
  → userHasSelected = true
  → URL updated with replace (no new history entry)
  → Back button returns to referrer, not prior span
```

### Why `replace` not `push`?

Span-to-span navigation within a trace is browsing, not destination-setting. If each span click pushed a history entry, back-button would step through every span the user glanced at instead of returning to the trace list. Note: Phase 6 uses `push` for user-driven filter changes and `replace` only for normalization. Here, `replace` is the right choice because span clicks are high-frequency inspection actions, not deliberate filter-setting actions that users would want to step back through.

### Stale Span Handling

If the selected span (from URL or state) is not found in the fetched span data:
1. Clear `selectedSpanExternalId`
2. Reset `userHasSelected = false`
3. Remove `?span=` from URL with `replace`
4. Let Phase 7 auto-selection re-run if applicable

This handles traces where spans are still being ingested or where a shared link references a span that was removed.

## 3. Search State Scoping

### Decision: Per-inspector, not page-global

Each `PayloadInspector` instance owns its own search state. The trace detail page may render 4+ inspectors simultaneously (trace input, trace output, span input, span output, metadata, timeline payload). A global search would be confusing because:

- Users typically search within a specific payload (e.g., "find the `temperature` key in the LLM request")
- Global search would need cross-inspector result navigation UI
- Match highlighting across collapsed inspectors creates UX complexity

### Search Behavior

- `useDeferredValue` defers the search rendering so that typing remains responsive while the tree re-renders with matches in a lower-priority update
- Matching branches auto-expand while search is active
- Clearing search restores prior manual expansion state (saved before search began)
- Search covers object keys and scalar values (stringified). No JSONPath in v1.
- Match navigation: next/previous buttons cycle through matches within that inspector. The active match gets a distinct highlight and is scrolled into view via `scrollIntoView`. DOM focus stays on the search input, not on the match element.

## 4. Unified Selection Callback

### Decision: Single page-level callback for all span selection

All entry points (tree row, breadcrumb, failure summary, parent button, timeline) call one shared callback. This callback:

1. Updates `selectedSpanExternalId` state
2. Sets `userHasSelected = true`
3. Computes and sets `revealPath` for tree auto-expand
4. Writes `?span=` with `replace`

This prevents divergence where some entry points forget URL updates or reveal-path computation. The callback lives in `TraceDetailPage.tsx` and is passed down via props.

## 5. Component Hierarchy

```
TraceDetailPage
├── FailureSummary (onSelectSpan → unified callback)
├── TraceContext
│   ├── PayloadInspector (trace input)
│   └── PayloadInspector (trace output)
├── SpanTree (onSelectSpan → unified callback)
├── SpanDetail
│   ├── SpanBreadcrumb (onSelectSpan → unified callback)
│   ├── Parent Span Button (→ unified callback)
│   ├── PayloadInspector (span input) + TruncationBanner
│   ├── PayloadInspector (span output) + TruncationBanner
│   ├── PayloadInspector (metadata)
│   └── CopyButton (span ID, parent ID)
├── Timeline
│   ├── Span buttons (onSelectSpan → unified callback)
│   └── PayloadInspector (event payload)
└── CopyButton (trace UUID, external trace ID, session ID, trace URL)
```
