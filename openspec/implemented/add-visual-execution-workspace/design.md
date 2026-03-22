## Context

The trace detail page currently renders span tree, span detail, and timeline in a vertical stack within a max-width container. Phase 9 transforms this into a professional debugger workspace with synchronized panels. The change is frontend-only and builds on existing foundations: `?span=` URL state, failure-first auto-selection, stale-trace detection, timeline polling, and payload inspection.

Key stakeholders: frontend users debugging traces on desktop; non-desktop users who need functional parity through tabs.

## Goals / Non-Goals

- Goals:
  - Desktop: left span tree rail, right workspace with waterfall on top and tabbed inspector below, resizable panels
  - Non-desktop: single stacked tabbed layout with `Waterfall`, `Tree`, `Details`, `Timeline`
  - Shared expansion model that initializes fully expanded (matching today's default)
  - Execution waterfall showing timing bars aligned with visible tree rows
  - Non-destructive tree search with match highlighting, ancestor auto-expand, and dim non-matches
  - All existing trace-detail behavior preserved (failure-first auto-selection, `?span=` deep-linking, timeline polling, payload inspection)

- Non-Goals:
  - Linked scroll synchronization between tree and waterfall
  - Custom keyboard shortcuts for panel navigation
  - Charting libraries (waterfall is built with React/Tailwind)
  - Adding a third-party virtualization library or linked-scroll system
  - URL-persisted panel sizes, tab state, or search queries

## Decisions

### State Architecture

Three-layer state split:
1. **Shared workspace state** (via `useWorkspaceState` hook): `selectedSpanExternalId`, `expandedSpanIds`, `revealPath`, `revealVersion`, `waterfallRevealTarget`, `userHasSelected`. Owned by the workspace shell, passed to all panels. The `userHasSelected` flag preserves the existing sticky-selection behavior from `TraceDetailPage.tsx`: once a user makes a deliberate selection (click, URL param), failure-first auto-selection does not override it. The flag resets on cross-trace navigation or when browser back/forward removes the `?span=` parameter. The `revealVersion` counter (monotonically incrementing) preserves the existing versioned-reveal signal from `TraceDetailPage.tsx` (`revealPathVersion`): every selection — including re-selecting the already-selected span — increments the counter so that tree ancestor expansion and scroll-into-view re-fire. Without this, re-selecting the same span after collapsing its ancestors would silently fail to reveal it.
2. **Tree-rail-local state**: `searchQuery`, deferred search model, `showMetrics` toggle. Owned by the tree rail component.
3. **Inspector-local state**: `activeInspectorTab`. Owned by the inspector, which exposes a `switchToDetails` callback upward.

**Why:** Shared selection/expansion must be consistent across tree, waterfall, and inspector. The `userHasSelected` flag is critical — without it, polling updates and timeline refreshes would re-trigger failure-first auto-selection and override deliberate user choices. Search and tab state are panel-local concerns that don't need cross-panel coordination and would add unnecessary complexity to the shared model.

### Panel Library

Use `react-resizable-panels` for desktop only.

**Alternatives considered:**
- Custom CSS Grid resizing: more control but significant implementation effort for drag handles, persistence, and accessibility.
- No resizing: limits user flexibility on wide monitors.

**Why:** `react-resizable-panels` is lightweight, well-maintained, handles accessibility, and avoids building resize mechanics from scratch.

### Waterfall Implementation

Build locally with React/Tailwind and shared time-scale helpers. No charting library.

**Why:** The waterfall is a single-axis bar chart. Adding a charting library (Recharts, D3) would introduce a large dependency for a simple visualization. Local implementation keeps the bundle small and allows tight integration with the shared row model.

### Large-Trace Windowing

Window the tree rail and waterfall row DOM with a local hook that renders the on-screen slice plus overscan, while preserving the full logical visible-row model for selection, expansion, reveal, and row-order synchronization.

**Why:** Local profiling showed that mounting every visible tree and waterfall row at once missed the Phase 9 interaction budgets on large traces. Windowing the off-screen rows keeps the user-visible behavior the same while bringing the shipped implementation back within budget. This remains frontend-local and does not introduce a new dependency beyond `react-resizable-panels`.

### Expansion Model

Replace per-node `useState(true)` in `SpanTreeNode` with a shared `expandedSpanIds: Set<string>` at workspace level. Initialize by collecting all span IDs with children so the tree renders fully expanded by default (matching today's behavior).

**Why:** Shared expansion is required so the waterfall can derive its visible rows from the same model. Per-node local state can't be observed by sibling panels.

### Tree Search

Non-destructive: dim non-matches rather than hiding them. Auto-expand matched ancestors. Restore prior expansion state when search clears.

**Why:** Hiding non-matches loses tree context. Dimming preserves hierarchy visibility while highlighting matches. Saving pre-search expansion and restoring on clear avoids the confusing "everything expanded after search" problem.

### Expand All Guard

Gate `Expand all` by projected visible-row expansion cost with a tunable constant, not an arbitrary span-count threshold. When the threshold is exceeded, use `window.confirm` for the confirmation dialog. This app does not currently have a modal/dialog pattern, and introducing one is out of scope for Phase 9.

**Why:** The cost of expanding depends on tree shape, not just total span count. A flat tree of 1000 spans is cheap; a deeply nested tree of 500 might be expensive. The constant should be validated by local profiling at 200, 400, 800, and 1200 spans before being locked. Using `window.confirm` avoids dragging in a modal system for a single guard button.

### Inspector Tab Retention

Keep `Details` and `Timeline` mounted while tabbed (CSS `display: none` for hidden tab).

**Why:** Prevents timeline-local state (e.g., error-only filter, scroll position) from being lost on tab switch.

**Performance fallback (not part of the shipped spec):** If profiling shows hidden Timeline DOM retention is too expensive on large traces, the fallback is unmounting the hidden `Timeline` tab and accepting state reset. Tree/waterfall row windowing is now part of the shipped Phase 9 design; Timeline retention remains the only documented fallback path.

### Failure Summary and Stale Trace Signal Relocation

Move from full-width page banners into the `Details` inspector tab as compact sections at the top.

**Why:** In the new layout, the tree rail and waterfall occupy the main viewport. Full-width banners would either push the workspace down or require complex layout adjustments. Placing them in the inspector keeps failure context immediately visible when viewing span details.

### Trace Context Collapse

Desktop: collapsed disclosure above the workspace. Non-desktop: collapsible section inside the `Details` tab.

**Why:** Trace Context metadata (IDs, tags, input/output) is important but secondary to the debugging workflow. Collapsing by default gives more viewport to the workspace. Desktop keeps it above the workspace for quick access; non-desktop puts it in the `Details` tab to avoid consuming a separate tab slot.

## Risks / Trade-offs

- **Large refactor surface**: Moving from a single file to a modular workspace touches many components. Mitigated by incremental PR delivery with stable intermediate states.
- **Performance on large traces**: Shared expansion state triggers full re-renders on collapse/expand. Mitigated by `useMemo` on visible-row derivation and explicit performance budgets.
- **react-resizable-panels as new dependency**: Adds ~8KB gzip. Acceptable for the functionality it provides; no other new dependencies.
- **Timeline DOM retention**: Keeping timeline mounted while hidden may use memory on large traces. Fallback plan is documented (unmount and accept state reset).

## Migration Plan

Incremental over 5 PRs:
1. PR 1: extract test file and shared helpers, no user-visible change
2. PR 2: desktop panel shell with tree rail, SpanDetail (top-right), Timeline (bottom-right)
3. PR 3: convert bottom-right into inspector tabs (Details + Timeline). SpanDetail stays in top-right. Details tab shows Failure Summary, Stale Trace Signal, and compact span summary. Full SpanDetail content remains top-right until PR 4.
4. PR 4: replace top-right SpanDetail with ExecutionWaterfall. Move full SpanDetail content into the Details tab. This completes the final layout.
5. PR 5: tree search, metrics hints, collapsed Trace Context, optional polish

Each PR is independently deployable. Rollback: revert the PR; no data migration needed.

## Open Questions

- None. All decisions have been locked per the proposal input.
