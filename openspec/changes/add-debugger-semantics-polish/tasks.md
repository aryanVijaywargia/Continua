## 1. Contract and codegen — Track A foundation

- [x] 1.1 Add `state_change` and `decision` to `IngestEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.2 Add `state_change` and `decision` to `TimelineEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.3 Run `make generate` and verify generated Go server, TypeScript, and Python SDK types (including `sdks/python/src/continua/types.py` event enums) all include new enum values

## 2. Backend event type handling — Track A

- [x] 2.1 Update `internal/ingest/processor.go` to accept `state_change` and `decision` event types. Add `log.Printf("[WARN] ...")` when `state_change` payload is missing `key` or `decision` payload is missing `question`/`chosen`. No changes to `ProcessedBatch` or response shape
- [x] 2.2 Add explicit `"state_change"` and `"decision"` cases to `mapExplicitTimelineEventType` in `internal/api/mapper.go` (prevents degradation to `TimelineEventTypeCustom`)
- [x] 2.3 Add Go tests: ingest processor accepts new event types, warnings fire for missing semantic fields, mapper maps correctly (not degraded to custom)

## 3. Python SDK helpers — Tracks A, C

- [x] 3.1 Add `span.state_change(key, old_value, new_value, namespace=None, message=None)` method to `sdks/python/src/continua/span.py`
- [x] 3.2 Add `span.decision(question, chosen, alternatives=None, reasoning=None, message=None)` method to `sdks/python/src/continua/span.py`
- [x] 3.3 Add docstrings with parameter descriptions and usage examples to both new methods
- [x] 3.4 Add Python SDK tests for `state_change()` and `decision()` helpers
- [x] 3.5 Run `cd sdks/python && uv run pytest` to verify all SDK tests pass

## 4. Frontend type updates — Track A

- [x] 4.1 Add `"state_change"` and `"decision"` to `TimelineEvent.event_type` union in `web/src/api/client.ts`
- [x] 4.2 Add `state_change` and `decision` cases to `summarizeTimelineEvent` in `web/src/utils/timeline.ts`. state_change: show `key: old → new` when payload fields present, fallback to `message`. decision: show `question → chosen` when payload fields present, fallback to `message`
- [x] 4.3 Update Timeline component rendering for new event types with fallback to generic event row when semantic fields are missing

## 5. State diff UI — Track B (depends on Track A)

- [x] 5.1 Create `web/src/utils/stateChanges.ts` with `extractStateChanges` utility that filters to `state_change` events where `payload.key` exists
- [x] 5.2 Create `web/src/components/StateDiffViewer.tsx` — grouped by namespace, scalar inline old→new, objects via `JsonViewer`
- [x] 5.3 Extend `InspectorTabs` to support a third "State" tab. Show badge count only when > 0 (no "(0)" noise). Update `InspectorTabId` type to include `"state"`
- [x] 5.4 Add `events?: TimelineEvent[]` prop to `SpanDetailProps` in `web/src/components/SpanDetail.tsx`. Render "Decisions" section filtered to selected span. Skip decision events where `payload.question` or `payload.chosen` is missing
- [x] 5.5 Thread timeline events from `TraceDetailPage` workspace to `SpanDetail` via the new `events` prop
- [x] 5.6 Update `WorkspaceShell` mobile tabs to include "State" as 5th tab. Update `MobileWorkspaceTabId` type
- [x] 5.7 Add Vitest tests for `extractStateChanges` utility and StateDiffViewer rendering
- [x] 5.8 Add integration-level Vitest tests for TraceDetailPage workspace with State tab and SpanDetail decisions section, and mobile workspace State tab parity

## 6. Event convention documentation — Track C

- [x] 6.1 Create `docs/event-conventions.md` with type table (all 8 explicit event types), payload schemas, SDK examples, and "debugger semantics, not replay primitives" scope guard

## 7. Settings UI and 401 recovery — Track D

- [x] 7.1 Create `web/src/pages/SettingsPage.tsx` using existing `getApiKey`/`setApiKey`/`clearApiKey` helpers. Show masked current key, input for new key, clear button
- [x] 7.2 Add `/settings` route to `App.tsx` within `PageWithNav`. Add "Settings" link to `Navigation`
- [x] 7.3 Update error rendering in `TracesPage`, `SessionsPage`, `SessionDetailPage`, `TraceDetailPage` to detect 401 via existing `ApiError.status` and render auth-error banner with "Go to Settings" link. Extract a shared helper only if the detection pattern becomes repetitive
- [x] 7.4 Add Vitest tests for 401 banner rendering and SettingsPage key management

## 8. Command palette — Track E

- [x] 8.1 Create `web/src/components/CommandPalette.tsx` with search input, filtered command list, arrow-key navigation, Enter to execute, Escape to dismiss. Commands: navigate Traces/Sessions/Settings, toggle theme
- [x] 8.2 Wire `CommandPalette` globally with `Cmd+K` / `Ctrl+K` shortcut. Guard against activation when text inputs, textareas, or contenteditable elements have focus. Add `⌘K` / `Ctrl+K` discoverability hint to `Navigation`
- [x] 8.3 Add Vitest tests for CommandPalette search filtering, keyboard navigation, command execution, and shortcut suppression when text inputs/textareas/contenteditable elements have focus

## 9. Dark mode and theming — Track F

- [x] 9.1 Enable `darkMode: 'class'` in `web/tailwind.config.js`
- [x] 9.2 Add CSS custom properties in `web/src/styles/globals.css` for non-Tailwind contexts (waterfall bar colors, panel separator colors) that change with `.dark` class
- [x] 9.3 Add pre-hydration theme script to `web/index.html` that reads localStorage and sets `dark` class on `<html>` before React renders
- [x] 9.4 Create `web/src/hooks/useTheme.ts` hook with `system | light | dark` modes, localStorage persistence, and OS preference detection via `prefers-color-scheme`
- [x] 9.5 Add theme toggle to `Navigation` component. Mirror theme selection on `SettingsPage`
- [x] 9.6 Apply `dark:` variants to shell components: `Navigation`, page layouts, cards, tables, inputs, `StatusBadge`
- [x] 9.7 Apply `dark:` variants to workspace components: `SpanTree`, `SpanDetail`, `Timeline`, `Waterfall`, `PayloadInspector`, `InspectorTabs`, `TreeRail`, `StateDiffViewer`
- [x] 9.8 Add Vitest test for `useTheme` hook (mode cycling, localStorage persistence)

## 10. Integration verification

- [x] 10.1 Run `make generate` — clean with no drift
- [x] 10.2 Run `go test ./internal/ingest/... -v` — new event types accepted, warnings fire
- [x] 10.3 Run `go test ./internal/api/... -v` — mapper tests pass
- [x] 10.4 Run `cd sdks/python && uv run pytest` — SDK tests pass
- [x] 10.5 Run `pnpm --filter web test` — all frontend tests pass

## Parallelism Notes

- Tasks 1–2 (contract + backend) are sequential and foundational
- Task 3 (Python SDK) can run after task 1 (needs contract types)
- Task 4 (frontend types) can run after task 1.3 (needs generated TS types)
- Tasks 5 (state diff UI) depends on task 4 (needs frontend event types)
- Tasks 6–9 (docs, settings, palette, dark mode) are independent of each other, can run in parallel after task 1.3
- Task 7.4 (401 banners) and 9.6–9.7 (dark variants) touch overlapping files — coordinate if parallelized
- Task 10 is final integration verification
