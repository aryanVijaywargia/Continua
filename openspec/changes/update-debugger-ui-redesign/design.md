## Context

The current Continua debugger already exposes the right product primitives: overview-worthy trace/session counts, URL-backed list pages, failure-first trace detail, session narratives, comparison, and a desktop/mobile workspace split. The redesign should improve hierarchy and ergonomics without changing backend APIs or invalidating the route/state model that current tests depend on.

## Goals

- Make the app feel like an operator console rather than a generic SaaS dashboard
- Improve scanability and actionability on traces, sessions, and detail pages
- Preserve the existing URL-backed behavior and investigation workflows
- Keep the redesign frontend-only and compatible with the current embedded SPA runtime

## Non-Goals

- Adding analytics APIs, dashboard time series, or new backend read models
- Reworking compare semantics or trace/session URL contracts
- Introducing agent graphs, eval workflows, or prompt playground features

## Decisions

### Decision: Shell-first information architecture

Use a shared shell with a left navigation rail and compact utility bar instead of independent page headers. This gives the product a stable frame and aligns Continua with current observability tools that prioritize route switching between overview, request/trace triage, and session investigation.

### Decision: Overview is a snapshot screen, not an analytics dashboard

The new `/` route will summarize counts and recent work using the existing list endpoints only. This keeps the redesign frontend-only and avoids fake charting driven by underpowered data.

### Decision: Utility-first visual language, not card mosaics

The redesign will remove the default "rounded white card everywhere" look. Most product surfaces become structured sections, list rows, split panes, toolbars, metric strips, and drawers. Cards remain only when a contained comparison or metric group benefits from them.

### Decision: Preserve current URL/state contracts

`/traces`, `/sessions`, `/sessions/:id`, `/sessions/:id/compare`, and `/traces/:id?span=` keep their existing state semantics. The redesign may change component composition and screen hierarchy, but it must not move core state out of search params or invent new public params.

### Decision: Desktop trace context becomes a drawer

Trace context is useful but secondary during active debugging. On desktop it moves out of the main workspace into a toggleable drawer. On mobile it stays accessible from the summary flow to avoid oversized chrome.

### Decision: Mobile trace detail reduces top-level tabs

The current six-tab mobile model spreads related information too thinly. The redesign collapses this to four top-level tabs:
- `Summary`: failure guidance, selected span detail, and reasoning
- `Execution`: tree/waterfall sub-toggle
- `Timeline`
- `State`

### Decision: Local quick filters only in the tree rail

Tree-rail quick filters will derive from existing loaded span data only, such as failed-only or kind-based filters. This improves investigation speed without requiring server-side filtering or new query keys.

## Implementation Notes

- Favor shared CSS tokens and a small number of reusable primitives over one-off style rewrites on every component
- Keep existing page labels and control names where tests or accessibility benefit from stability
- Where possible, restyle and recompose existing components rather than replacing their underlying data flow
- Keep desktop/mobile behavior deterministic for tests; avoid motion or viewport behavior that depends on timing
