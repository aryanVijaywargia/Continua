# Change: Add Debugger Semantics & Final Polish

## Why
The debugger has traces, sessions, span trees, waterfall, timeline, payload inspection, failure-first UX, and session-scale browsing (Phases 1–10). Before replay begins, two gaps remain: (1) the event taxonomy lacks structured state/decision semantics — state changes and decision points are stored as opaque `custom` events, preventing meaningful rendering; (2) the debugger shell lacks polish expected of a daily-use tool — no dark mode, no keyboard-driven navigation, no settings management, and no 401 error recovery.

Phase 11 closes these gaps with six capabilities: formal event taxonomy, state diff UI, event convention documentation, settings management, command palette, and dark mode.

## What Changes
- **Event taxonomy**: `state_change` and `decision` added to both `IngestEventType` and `TimelineEventType` OpenAPI enums. Ingest processor accepts them unconditionally; missing semantic fields (`key` for state_change, `question`/`chosen` for decision) produce server-log warnings via `log.Printf` but never reject the event. Mapper explicitly maps both types to prevent degradation to `custom`. Python SDK gains `span.state_change()` and `span.decision()` helpers. Frontend TypeScript types updated. Timeline renders full semantic UI when payload fields are present, degrades to generic event row using `message` when they are absent.
- **State diff UI**: New `StateDiffViewer` component grouped by namespace with scalar inline and objects via existing `JsonViewer`. New `extractStateChanges` utility filters to events where `payload.key` exists. "State" tab added to `InspectorTabs` (badge count only when > 0). `SpanDetail` gains an `events` prop and renders a "Decisions" section filtered to the selected span. `WorkspaceShell` mobile tabs gain a 5th "State" tab.
- **Event conventions**: New `docs/event-conventions.md` documenting type table, payload schemas, SDK examples. Explicit scope guard: "debugger semantics, not replay primitives." Python SDK docstrings enhanced with usage examples.
- **Settings UI**: New `SettingsPage` using existing `getApiKey`/`setApiKey`/`clearApiKey` helpers. Route and nav link added. 401 error recovery: pages detect 401 via the existing `ApiError.status` field and render auth-error banners with "Go to Settings" link. No toast system — uses existing banner pattern.
- **Command palette**: New `CommandPalette` component with search, arrow-key navigation, Enter/Escape. Commands: navigate to Traces/Sessions/Settings, toggle theme. Wired to `Cmd+K`/`Ctrl+K` globally. Discoverability hint in Navigation.
- **Dark mode**: Tailwind `darkMode: 'class'` enabled. CSS custom properties in `globals.css` only for non-Tailwind contexts (waterfall inline styles, panel separators). Pre-hydration theme script prevents FOUC. `useTheme` hook with `system | light | dark` persisted to localStorage. Theme toggle in Navigation and Settings. `dark:` variants applied to all shell and workspace components.

## Scope Boundary
- No replay, checkpoints, resumability
- No server-side settings CRUD or project-admin settings
- No API-key rotation
- No orphan-event surfacing (deferred — build only if users report confusion)
- No changes to ingest response shape or `ProcessedBatch` return path

## Impact
- Affected specs: `event-taxonomy`, `state-diff-ui`, `event-conventions`, `settings-ui`, `command-palette`, `dark-mode`
- Affected code:
  - Backend: `contracts/openapi/openapi.yaml`, `internal/ingest/processor.go`, `internal/api/mapper.go`
  - Python SDK: `sdks/python/src/continua/span.py`
  - Frontend: `web/src/api/client.ts`, `web/src/utils/timeline.ts`, `web/src/components/Timeline.tsx`, `web/src/components/InspectorTabs.tsx`, `web/src/components/SpanDetail.tsx`, `web/src/pages/TraceDetailPage.tsx`, `web/src/components/WorkspaceShell.tsx`, `web/src/App.tsx`, `web/src/components/Navigation.tsx`, `web/tailwind.config.js`, `web/src/styles/globals.css`, all page and workspace components (dark variants)
  - New files: `web/src/components/StateDiffViewer.tsx`, `web/src/utils/stateChanges.ts`, `web/src/components/CommandPalette.tsx`, `web/src/pages/SettingsPage.tsx`, `web/src/hooks/useTheme.ts`, `docs/event-conventions.md`
- No new runtime dependencies
