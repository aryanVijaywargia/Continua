# Change: Update Debugger UI Redesign

## Why

Continua's debugger already has solid trace, session, and timeline behavior, but the current UI reads like a collection of generic Tailwind pages rather than a deliberate observability product. Operators need a calmer information hierarchy, stronger navigation, denser triage surfaces, and more intentional workspaces so they can move from overview to trace/session investigation without friction.

## What Changes

- Add a shared operator-first application shell with a left navigation rail, compact top utility bar, API-key status, and route-aware layout for overview, traces, sessions, settings, and deep detail pages.
- Replace the placeholder `/` route with a real overview screen built only from existing trace and session endpoints, emphasizing snapshot counts, running/failing work, and recent investigation entry points.
- Redesign traces and sessions into denser triage surfaces with sticky filter/search controls, quick filters, better row hierarchy, and shared loading/empty/error states while preserving existing URL-backed search state.
- Redesign session detail and compare pages into clearer operator workspaces with stronger summary/header structure and more deliberate compare affordances, without changing compare URL semantics.
- Redesign trace detail into a denser investigation workspace: stronger header, slimmer tree rail, refined waterfall/inspector presentation, desktop trace-context drawer, mobile four-tab composition, and local tree-rail quick filters using existing span data only.
- Introduce a cohesive visual system across the web app using embedded IBM Plex fonts, graphite/ivory tokenized surfaces, restrained shadows, and semantic status color reserved for state and failures.
- Add frontend coverage for the shell, overview, redesigned list layouts, mobile workspace composition, and trace/session navigation continuity.
- Add Playwright screenshot smoke coverage scaffolding for the main routes against seeded demo data.

## Scope Limits

- Frontend-only redesign. No REST contract, database, backend behavior, or SDK changes.
- Overview metrics are derived from existing list endpoints only. No new analytics or time-series APIs.
- Existing routes remain intact; only `/` changes from placeholder landing page to real overview.
- Existing URL params remain authoritative on traces, sessions, session detail, compare, and `?span=` trace selection.
- No new public URL params in this phase.
- No new product capabilities such as evals, annotations, agent graphs, prompt playgrounds, or replay flows.

## Impact

- Affected specs: `debugger-ui-redesign` (new capability)
- Affected code:
  - `web/src/App.tsx` and new shared shell/layout components
  - `web/src/styles/globals.css` and related UI primitives
  - `web/src/pages/TracesPage.tsx`
  - `web/src/pages/SessionsPage.tsx`
  - `web/src/pages/SessionDetailPage.tsx`
  - `web/src/pages/SessionComparePage.tsx`
  - `web/src/pages/TraceDetailPage.tsx`
  - `web/src/pages/SettingsPage.tsx`
  - targeted shared components under `web/src/components/`
- New dependency: embedded IBM Plex font packages and Playwright test tooling
