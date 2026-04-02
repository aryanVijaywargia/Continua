# Continua Debugger Platform Baseline

> **Status: Current**
> This is the shortest repo-verified handoff for the current checkout. Use [README.md](../README.md) for setup, [docs/README.md](./README.md) for doc status, and [PHASE5_CURRENT_STATE_REPORT.md](./PHASE5_CURRENT_STATE_REPORT.md) only as deeper historical context.

Status date: 2026-04-02

Purpose: give future agents a short, repo-verified baseline for the current product state after the debugger shell/overview/session-compare redesign landed in the working tree.

## Summary

Continua's live product today is an AI agent observability debugger built on one implemented path:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger operator console
```

The observability core is already real:
- project-scoped authenticated ingest
- durable idempotent batch handling
- true async ingest infrastructure
- trace rollups
- trace, session, and timeline read APIs
- session compare API
- Python SDK helpers for traces, spans, sessions, and async batch polling

The debugger surface is also real:
- shared `AppShell` with route-aware shell navigation
- overview route at `/` built from existing trace and session endpoints only
- traces list with URL-driven filter, sort, and pagination state
- sessions list and session detail with URL-driven state
- session compare workspace at `/sessions/:id/compare`
- failure-first trace detail flow
- trace workspace with tree rail, execution waterfall, inspector tabs, and desktop trace-context drawer
- payload inspection, truncation banners, breadcrumb navigation, and span deep links
- state-change and decision event semantics
- settings page, auth recovery, command palette, theming, and operator-console styling
- Playwright smoke-test scaffolding for the major routes

## Implemented Vs Future

Implemented and active:
- `internal/api`
- `internal/ingest`
- `internal/jobs`
- `internal/store`
- `internal/config`
- `internal/web`
- `web/src`
- `sdks/python`

Still scaffolded or future-facing:
- `engine/`
- `internal/proxy`
- `internal/ws`
- `internal/replay`
- `internal/alerts`
- `internal/export`
- `internal/state`
- `internal/telemetry`
- `sdks/typescript`

Do not describe replay, live WebSocket runtime, proxy capture, score APIs, or TypeScript SDK parity as implemented.

## Debugger Frontend Shape

Current routes:
- `/`
- `/traces`
- `/traces/:id`
- `/sessions`
- `/sessions/:id`
- `/sessions/:id/compare`
- `/settings`

Important frontend patterns:
- the app is wrapped in a shared `AppShell` that owns primary navigation, shell chrome, API-key state, theme toggle, and command palette access
- overview remains frontend-only and uses existing list endpoints rather than new analytics APIs
- list-page state lives in URL params, not local-only component state
- session compare state lives in `baseline_trace_id` / `candidate_trace_id` query params
- trace detail span selection is URL-backed via `?span=`
- running traces poll `/api/traces/{id}/events`; there is no live WebSocket runtime
- desktop trace detail keeps the main workspace in `WorkspaceShell` and moves trace context into a toggleable drawer
- mobile trace detail uses `Summary`, `Execution`, `Timeline`, and `State` top-level tabs, with a tree/waterfall sub-toggle inside `Execution`
- `TreeRail` now supports local quick filters that work only on already loaded span data
- complex UI logic is factored into pure helpers and small hooks such as `useWorkspaceState`, `useTraceDetailSearchParams`, `spanTree.ts`, and `waterfallTime.ts`

## Backend And Data Model Reality

Source-of-truth inputs:
- REST contract: `contracts/openapi/openapi.yaml`
- WebSocket schema contract: `contracts/websocket/events.ts`
- platform schema and queries: `db/platform/migrations/postgres/`, `db/platform/queries/`
- runtime behavior: `cmd/continua`, `internal/`, `web/`, `sdks/python/`

Important persisted entities:
- `projects`
- `ingest_batches`
- `ingest_batch_payloads`
- `sessions`
- `traces`
- `spans`
- `span_events`

Important semantics:
- `sessions.external_id` is the user-facing session identifier
- `traces.trace_id` is the external trace identifier
- `spans.span_id` and `spans.parent_span_id` are external span identifiers
- session compare reads terminal traces within a session through `GET /api/sessions/{id}/compare`
- timeline responses merge explicit events with synthetic lifecycle markers

## Validation Surfaces

Current web validation surfaces include:
- `pnpm --filter web test`
- `pnpm --filter web test:e2e`
- `web/playwright.config.ts`
- `web/e2e/ui-smoke.spec.ts`

## OpenSpec State

Repo convention:
- active proposals belong in `openspec/changes/`
- completed change history belongs in `openspec/implemented/`
- `openspec/specs/` is still empty, so OpenSpec is not the complete current-state source of truth

Active change material may describe the debugger redesign as in flight, but the current checkout already contains the relevant shell/overview/session-compare UI code. For current-state truth, prefer live code plus this baseline.

## Planning Guidance

Use this baseline when starting new debugger work:
- extend the debugger UI from the existing shell, overview, session, compare, and trace workspaces rather than rebuilding them from scratch
- preserve the current URL-driven state patterns on traces, sessions, and trace detail
- preserve current session compare URL semantics and trace-detail return navigation
- treat the React debugger as the active product surface and the Python SDK as the active ingestion surface
- treat `docs/PHASE5_CURRENT_STATE_REPORT.md` as deep historical context, not the shortest current-state handoff
