# Continua Debugger Platform Baseline

Status date: 2026-03-22

Purpose: give future agents a short, repo-verified baseline for the current product state after the observability buildout and before new debugger-platform work begins.

## Summary

Continua's live product today is an AI agent observability debugger built on one implemented path:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger UI
```

The observability core is already real:
- project-scoped authenticated ingest
- durable idempotent batch handling
- true async ingest infrastructure
- trace rollups
- trace, session, and timeline read APIs
- Python SDK helpers for traces, spans, sessions, and async batch polling

The debugger surface is also real:
- traces list with URL-driven filter, sort, and pagination state
- sessions list and session detail with URL-driven state
- failure-first trace detail flow
- trace workspace with tree rail, execution waterfall, and inspector tabs
- payload inspection, truncation banners, breadcrumb navigation, and span deep links
- state-change and decision event semantics
- settings page, auth recovery, command palette, and theming

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
- `/traces`
- `/traces/:id`
- `/sessions`
- `/sessions/:id`
- `/settings`

Important frontend patterns:
- list-page state lives in URL params, not local-only component state
- trace detail span selection is URL-backed via `?span=`
- running traces poll `/api/traces/{id}/events`; there is no live WebSocket runtime
- `TraceDetailPage` coordinates `TreeRail`, `ExecutionWaterfall`, `InspectorTabs`, `SpanDetail`, `Timeline`, and `StateDiffViewer`
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
- timeline responses merge explicit events with synthetic lifecycle markers

## OpenSpec State

Repo convention:
- active proposals belong in `openspec/changes/`
- completed change history belongs in `openspec/implemented/`
- `openspec/specs/` is still empty, so OpenSpec is not the complete current-state source of truth

Debugger-oriented implemented history now lives in:
- `openspec/implemented/add-trace-discovery-triage`
- `openspec/implemented/add-failure-first-trace-detail`
- `openspec/implemented/add-payload-inspection-deep-navigation`
- `openspec/implemented/add-visual-execution-workspace`
- `openspec/implemented/add-session-scale-ux`
- `openspec/implemented/add-debugger-semantics-polish`

Still active:
- `openspec/changes/add-true-async-ingest`
  - feature implementation is largely present, but metrics and some validation items are still open in its task list

## Planning Guidance

Use this baseline when starting new debugger work:
- extend the debugger UI from the existing trace workspace rather than rebuilding trace detail from scratch
- preserve the current URL-driven state patterns on traces, sessions, and trace detail
- treat the React debugger as the active product surface and the Python SDK as the active ingestion surface
- treat `docs/PHASE5_CURRENT_STATE_REPORT.md` as deep historical context, not the shortest current-state handoff
