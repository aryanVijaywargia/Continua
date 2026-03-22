# Tasks: Payload Inspection & Deep Navigation

## 1. Payload Inspector Foundation

### 1.1 Build payload tree data structure
- [x] Create tree node types (`ObjectNode`, `ArrayNode`, `PrimitiveNode`) with `depth`, `key`, `path`, `childCount`
- [x] Implement `buildPayloadTree(data: unknown): TreeNode` with total node counting
- [x] Apply initial expansion rules: depth >= 2 collapsed, > 200 children collapsed at depth 0-1
- [x] Unit test: primitives, objects, arrays, nulls, empty collections, deep nesting, wide arrays

### 1.2 Build PayloadInspector component
- [x] Create `PayloadInspector.tsx` with expand/collapse toggle per node
- [x] Render count badges for collapsed objects/arrays
- [x] Render JSON `null` as a `PrimitiveNode` showing literal `null`
- [x] Render `undefined` (absent field) with "No data" placeholder
- [x] Multiline string rendering with `white-space: pre-wrap` and bounded height
- [x] Long single-line string wrapping
- [x] Keyboard activation (Enter/Space) for expand/collapse toggles

### 1.3 Add inspector toolbar
- [x] Toolbar with: search input, next/prev match, expand all, collapse all, copy full JSON
- [x] Gate "Expand all" when total node count > 5,000
- [x] "Collapse all" restores initial expansion state

### 1.4 Add per-inspector search
- [x] Search over object keys and stringified scalar values
- [x] `useDeferredValue` to defer tree re-rendering during search input
- [x] Auto-expand ancestor branches for matches
- [x] Match count display and next/previous navigation
- [x] Active match gets distinct highlight style; other matches get standard highlight
- [x] Active match scrolled into view via `scrollIntoView`; DOM focus stays on search input
- [x] Clearing search restores pre-search expansion state
- [x] Unit test: key matches, value matches, match ordering, no-match case, active-match highlight distinct from other matches, active match scrolled into view

## 2. Span Deep-Linking

### 2.1 Create URL param utilities
- [x] Create `traceDetailSearchParams.ts` with `parseSpanParam` and `serializeSpanParam`
- [x] Preserve unrelated query params
- [x] Unit test: present span, empty span (normalized to null), missing span, unrelated params preserved

### 2.2 Create useTraceDetailSearchParams hook
- [x] Create `useTraceDetailSearchParams.ts` wrapping `useSearchParams`
- [x] Expose parsed `spanParam` (raw string or null) and `setSpanParam(id | null)` writer
- [x] Writer preserves unrelated query params and uses `replace`
- [x] Hook does NOT perform semantic validation (no spanIndex dependency)
- [x] Follow Phase 6 hook patterns

### 2.3 Integrate URL span state into TraceDetailPage
- [x] Read `spanParam` from hook and validate against `spanIndex` after span data loads
- [x] On valid span: set `selectedSpanExternalId`, set `userHasSelected = true`
- [x] On unknown span (after data available): remove param via hook, reset `userHasSelected`, re-run auto-selection
- [x] React to `spanParam` changes while mounted (browser back/forward, manual URL edits)
- [x] Write `?span=` with `replace` on explicit user selection (via unified callback)
- [x] Do NOT write URL on Phase 7 auto-selection
- [x] Handle stale span on polling refresh: clear selection, remove param, reset `userHasSelected`, re-run auto-selection
- [x] Verify React Query keys do not include `?span=` (no refetch on span change)
- [x] Integration test: load with valid span, load with unknown span, span sticky across polling, stale span fallback
- [x] Integration test: browser back changes `?span=` to a different span within same trace → selection updates
- [x] Integration test: browser back removes `?span=` entirely within same trace → selection clears, `userHasSelected` resets, auto-selection re-runs

## 3. Clipboard Utilities

### 3.1 Create clipboard helper
- [x] Create `clipboard.ts` with `copyToClipboard(text: string): Promise<void>`
- [x] Handle clipboard API unavailability with rejection
- [x] Unit test: success and failure paths

### 3.2 Create CopyButton component
- [x] Create `CopyButton.tsx` with transient success/error feedback (~2s)
- [x] Keyboard activation (Enter/Space)
- [x] Accept `value: string` or `getValue: () => string` prop

### 3.3 Add copy targets to trace detail page
- [x] Trace identifiers: internal UUID, external trace ID, session ID
- [x] "Copy Trace URL" button constructing absolute URL with effective selected span
- [x] Integration test: Copy Trace URL includes effective span
- [x] Integration test: Copy Trace URL preserves existing non-span params (e.g., `debug=true`)

## 4. Navigation Continuity

### 4.1 Unify selection callback
- [x] Create single `handleSelectSpan` in TraceDetailPage that: updates state, sets `userHasSelected`, computes `revealPath`, writes `?span=`
- [x] Wire all entry points through it: span tree, breadcrumb, failure summary, timeline
- [x] Fix timeline navigability: render span button when `span_id` resolves in `spanIndex` (regardless of `span_name` presence); use `span_name` as label with `span_id` fallback
- [x] Integration test: all entry points keep tree/detail/URL synchronized

### 4.2 Add parent span navigation
- [x] Render parent span ID as button when parent exists in span index
- [x] Render as plain text when non-null `parent_span_id` does not resolve in span index
- [x] Show no parent row when `parent_span_id` is null or absent (root span)
- [x] Unit test: root span (null parent) → no parent row; unresolvable parent → plain text; resolvable parent → button
- [x] Wire through unified selection callback
- [x] Keyboard activation (Enter/Space)

### 4.3 Add span identifier copy controls
- [x] CopyButton for span external `span_id` in SpanDetail
- [x] CopyButton for parent span ID in SpanDetail (when present)

## 5. Truncation Indicators & Inspector Rollout

### 5.1 Add truncation banner component
- [x] Create truncation banner rendering: truncated flag, original size (human-readable), reason
- [x] Handle partial metadata (flag only, flag + size, full metadata)
- [x] Format sizes: bytes → KB/MB/GB
- [x] Unit test: full metadata, partial metadata, not truncated, explicit false

### 5.2 Wire truncation banners to span payloads
- [x] Show above span input PayloadInspector when `input_truncated`
- [x] Show above span output PayloadInspector when `output_truncated`
- [x] Do NOT show for trace-level or timeline payloads
- [x] Integration test: banner renders for span payloads only

### 5.3 Roll out PayloadInspector to all surfaces
- [x] Trace context input/output
- [x] Span detail input/output/metadata
- [x] Timeline event payload
- [x] Remove or reduce `JsonViewer.tsx` to thin wrapper (no divergent renderers)

## 6. Final Verification

### 6.1 Cross-phase regression integration tests
- [x] Integration test: navigate from filtered trace list (`/traces?status=failed`) → trace detail → select span → press back → lands on filtered trace list (Phase 6 back-link preserved)
- [x] Integration test: load `/traces/:id` (no `?span=`) for a failed trace → Phase 7 auto-selects primary failed span → URL does NOT contain `?span=`
- [x] Integration test: navigate from trace A with `?span=x` → trace B (different `traceId`) → `selectedSpanExternalId` resets to `null`, `userHasSelected` resets to `false`, `?span=` is absent (Phase 7 cross-trace reset)
- [x] Integration test: Phase 7 failure-first auto-selection + Phase 8 URL span coexistence — load with `?span=abc` on a failed trace → auto-selection does NOT override URL-driven selection

### 6.2 Keyboard activation
- [x] Inspector expand/collapse toggles respond to Enter/Space
- [x] Copy buttons respond to Enter/Space
- [x] Parent span button responds to Enter/Space

### 6.3 Type checking and lint
- [x] `make type-check` passes
- [ ] `make lint` passes (`golangci-lint` remains blocked locally: the available binary reports Go 1.23 while the repo targets Go 1.24.11)
- [x] All new tests pass
