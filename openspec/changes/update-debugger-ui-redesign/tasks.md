## 1. OpenSpec

- [ ] 1.1 Add `proposal.md`, `tasks.md`, `design.md`, and `specs/debugger-ui-redesign/spec.md`
- [ ] 1.2 Validate the change with `openspec validate update-debugger-ui-redesign --strict`

## 2. Shared App Shell and Visual System

- [ ] 2.1 Add embedded IBM Plex Sans and IBM Plex Mono to the web app
- [ ] 2.2 Replace the current top-only navigation wrapper with a shell that provides a desktop left rail, compact top utility bar, and mobile drawer navigation
- [ ] 2.3 Surface route-aware navigation items for `Overview`, `Traces`, `Sessions`, and `Settings`
- [ ] 2.4 Add API-key presence status and keep theme + command palette actions in the shell
- [ ] 2.5 Introduce shared operator-console tokens and reusable surface/button/input styles in `globals.css`

## 3. Overview Route

- [ ] 3.1 Replace the placeholder home page with a real overview page
- [ ] 3.2 Build overview snapshot metrics using existing trace and session list endpoints only
- [ ] 3.3 Add sections for recent failures, running traces, recent sessions, and direct jump actions
- [ ] 3.4 Add empty/loading/error/auth states aligned with the new visual system

## 4. Traces and Sessions Triage

- [ ] 4.1 Redesign the traces page into a sticky-toolbar triage surface while preserving existing URL search-param behavior
- [ ] 4.2 Add quick-filter affordances and denser trace rows with stronger name/status emphasis
- [ ] 4.3 Redesign the sessions page into a workflow/session index while preserving existing search/sort/pagination behavior
- [ ] 4.4 Replace the plain sessions table treatment with denser rows and clearer secondary scan data
- [ ] 4.5 Update shared pagination and sortable controls to match the new system

## 5. Session Workspaces

- [ ] 5.1 Redesign the session detail header and summary surfaces
- [ ] 5.2 Recompose narrative/storyline and trace table into a clearer operator workspace without changing compare URL semantics
- [ ] 5.3 Redesign compare selection surfaces and action hierarchy
- [ ] 5.4 Redesign the session compare page with a persistent baseline/candidate header and clearer diff grouping

## 6. Trace Workspace

- [ ] 6.1 Redesign the trace detail header and summary strip
- [ ] 6.2 Move desktop trace context into a right-side drawer while keeping mobile expandable access
- [ ] 6.3 Refine tree rail, waterfall, inspector tabs, and span detail presentation for denser operator use
- [ ] 6.4 Add local tree-rail quick filters using existing span data only
- [ ] 6.5 Reduce mobile workspace composition to `Summary`, `Execution`, `Timeline`, and `State`, with execution sub-toggle between tree and waterfall

## 7. Settings and Auth Surface

- [ ] 7.1 Redesign the settings page into the same shell/system language
- [ ] 7.2 Redesign the API key prompt to match the operator-console visual system

## 8. Tests and Validation

- [ ] 8.1 Add or update Vitest coverage for the app shell and overview route
- [ ] 8.2 Update traces and sessions page tests to cover the redesigned layouts without regressing URL-state behavior
- [ ] 8.3 Update trace detail tests for the desktop context drawer and the new four-tab mobile composition
- [ ] 8.4 Add coverage for session detail/compare layout continuity where the redesign changes semantics or visible controls
- [ ] 8.5 Add Playwright screenshot smoke tests/config for overview, traces, trace detail, sessions, session detail, session compare, and settings using seeded demo data
- [ ] 8.6 Run `pnpm --filter web test`
- [ ] 8.7 Run targeted build/type-check validation for the web app
