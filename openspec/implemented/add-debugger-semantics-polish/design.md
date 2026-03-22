# Design: Debugger Semantics & Final Polish

## Overview
Phase 11 spans backend contract changes, SDK updates, and significant frontend work across six capabilities. This document captures cross-cutting architectural decisions.

## Dependency Structure

```
Track A: Event Taxonomy (contract + backend + SDK + frontend types)
  ↓
Track B: State Diff UI (depends on A's new event types)

Tracks C–F are independent of each other, parallelizable after A:
  Track C: Event Convention Docs
  Track D: Settings UI + 401 Recovery
  Track E: Command Palette
  Track F: Dark Mode / Theming
```

Track A is foundational — it modifies the OpenAPI contract, which triggers `make generate` and cascades to backend, SDK, and frontend types. Track B depends on A because `StateDiffViewer` and the "Decisions" section require the new `state_change` and `decision` event types to exist in the type system.

## Key Architectural Decisions

### 1. Payload Validation: Accept-and-Warn

**Decision**: Accept `state_change`/`decision` events even when semantic payload fields are missing. Log warnings server-side via `log.Printf("[WARN] ...")`.

**Why**: Rejecting events for missing optional payload fields would break the established ingest contract where payload is always optional. The existing pattern (processor.go) accepts any valid event type and stores whatever payload is provided. Validation strictness belongs in the SDK (which will populate fields correctly) and in the UI (which degrades gracefully).

**Trade-off**: Garbage-in is possible. Mitigated by: (a) SDK helpers enforce correct payload shape; (b) UI falls back to generic rendering; (c) `docs/event-conventions.md` documents expected schemas.

### 2. Mapper Explicit Mapping (A3)

**Decision**: Mapper must explicitly map `state_change` → `TimelineEventTypeStateChange` and `decision` → `TimelineEventTypeDecision`.

**Why**: The current `mapExplicitTimelineEventType` in `mapper.go:353` has a `default: return TimelineEventTypeCustom` fallback. Without explicit cases, new event types silently degrade to `custom` and the frontend never sees them. This was identified as a critical gap — without A3, Tracks A6 and B are broken.

### 3. Theming: Tailwind dark: Classes Primary

**Decision**: Use Tailwind `darkMode: 'class'` as the primary mechanism. CSS custom properties in `globals.css` only for contexts where Tailwind classes cannot reach (inline styles on waterfall bars, resizable panel separators).

**Why**: The codebase is 100% Tailwind utility classes today. Introducing a parallel CSS variable system for everything would create maintenance burden. CSS vars are limited to the two known inline-style contexts: waterfall bar colors (computed widths/colors in JSX) and panel separator styling.

**FOUC prevention**: A synchronous `<script>` in `index.html` reads localStorage before React hydrates and sets the `dark` class on `<html>`. This runs before any paint.

### 4. Settings: No ApiKeyPrompt Refactor

**Decision**: `SettingsPage` uses `getApiKey`/`setApiKey`/`clearApiKey` directly. The existing `ApiKeyPrompt` component (used for first-run) is not refactored or reused.

**Why**: `ApiKeyPrompt` has first-run-specific UX (welcome copy, redirect logic). Reusing it in Settings would require conditional rendering that adds complexity for no UX gain. A simple form with clear/update is cleaner.

### 5. 401 Recovery: Page-Level Banners

**Decision**: 401 errors show a page-level auth-error banner with "Invalid or missing API key" and a "Go to Settings" link. No toast system.

**Why**: The existing error rendering paths on `TracesPage`, `SessionsPage`, `SessionDetailPage`, and `TraceDetailPage` already render full-page error states. Adding a 401-specific variant is consistent. A toast system would be a new pattern with no other current use case — over-engineering for a single error type.

**Implementation note**: The existing `ApiError` class already has a `status` field, so 401 detection can use `error instanceof ApiError && error.status === 401` directly. A dedicated `isAuthError` helper is optional — use it if the check becomes repetitive across pages, but the spec does not require a specific abstraction. Each page's existing error render path adds a 401 check before the generic error fallback.

### 6. SpanDetail Events Threading (B4)

**Decision**: Thread timeline events to `SpanDetail` via a new `events` prop on `SpanDetailProps`.

**Why**: `SpanDetail` currently has no access to events — it only receives the `Span` object. To render a "Decisions" section filtered to the selected span, it needs the event list. The parent (`TraceDetailPage` workspace) already has timeline events loaded; passing them down is the simplest path.

**Filter**: `SpanDetail` filters to `event.span_id === span.span_id && event.event_type === 'decision'` and skips entries where `question` or `chosen` is missing from the payload.

### 7. Command Palette Scope

**Decision**: Minimal command set — navigate to Traces/Sessions/Settings, toggle theme. No fuzzy search over traces/spans/sessions.

**Why**: Phase 11 is about shell polish, not a power-user search engine. Data-aware search (find a trace by name) requires API integration and is a separate feature. The palette establishes the UI pattern and keyboard shortcut; commands can be extended later.

## Preservation Concerns

These existing behaviors must not regress:
- Filtered back-navigation and `?span=` deep-link selection
- Failure-first auto-selection of primary failed span
- Timeline polling and stale trace detection
- Payload inspection with search, expand/collapse, copy
- Resizable 3-panel workspace layout
- Mobile tab-switcher fallback
- Sessions URL-driven search/filter/sort
- Trace/session pagination and `returnTo` navigation
