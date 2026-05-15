# Architecture Overview

> **Status: Current**
> This document summarizes the current runtime shape.

## Current Runtime

Continua's implemented platform is a single Go server plus an embedded React debugger operator console.

The active request path today is:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> React debugger operator console
```

## Runtime Components

### Platform Server

The Go server in `cmd/continua` is the real product runtime today. It is wired with Fx in `cmd/continua/main.go` and provides:
- authenticated REST APIs from `contracts/openapi/openapi.yaml`
- ingest handling in `internal/api` + `internal/ingest`
- River worker startup in `internal/jobs`
- Postgres-backed store/query access in `internal/store`
- the embedded web UI from `internal/web`

The HTTP router is Chi-based in `internal/api/router.go`.

### Web UI

The frontend in `web/` is a Vite React SPA built into `internal/web/static/` and embedded into the Go binary.

Implemented UI areas include:
- shared `AppShell` with primary navigation and route-aware utility chrome
- `/` overview built from existing trace and session list endpoints only
- traces list with URL-driven filtering and return navigation
- trace detail with failure-first triage, a desktop trace-context drawer, and mobile `Summary` / `Execution` / `Timeline` / `State` tabs
- local tree-rail quick filters that operate only on already loaded span data
- payload inspection, truncation banners, reasoning/state surfaces, and merged polling-based timeline
- sessions list, session detail, and session compare workspaces
- settings, auth recovery, command palette, theming, and operator-console visual styling

### Background Jobs

River workers run inside the platform server and currently handle:
- async ingest batch processing
- trace rollup computation
- failed payload cleanup

### SDKs

Current state:
- `sdks/python/`: implemented and usable
- `sdks/typescript/`: stub package only

## Internal Package Map

### Actively extended packages
- `internal/api`
- `internal/config`
- `internal/ingest`
- `internal/jobs`
- `internal/store`
- `internal/web`

### Mostly scaffolded packages
- `internal/proxy`
- `internal/ws`
- `internal/replay`
- `internal/alerts`
- `internal/export`
- `internal/state`
- `internal/telemetry`

## Data Flow

### Ingest

```text
POST /v1/ingest
  -> API key auth resolves project scope
  -> request validation
  -> batch idempotency claim
  -> sync write path or async acceptance
  -> trace rollup jobs
```

### Debugger Reads

```text
GET /api/traces
GET /api/traces/{id}
GET /api/traces/{id}/spans
GET /api/traces/{id}/events
GET /api/sessions
GET /api/sessions/{id}
GET /api/sessions/{id}/compare
```

The trace detail UI does not use a live WebSocket runtime today. It polls the timeline API for running traces and merges explicit stored events with synthetic lifecycle events derived from spans.

## Storage Model

The important persisted entities today are:
- `projects`
- `ingest_batches`
- `ingest_batch_payloads`
- `sessions`
- `traces`
- `spans`
- `span_events`

Important semantics:
- sessions expose internal UUIDs plus user-facing `external_id`
- traces expose internal UUIDs plus external `trace_id`
- spans expose internal UUIDs plus external `span_id`
- span tree parent links use external `parent_span_id`
- trace and span payloads live on `traces` / `spans`; there is no active standalone runtime payload table
- timeline responses merge explicit events with synthetic lifecycle entries

## Contracts And Generation

Source-of-truth files:
- REST: `contracts/openapi/openapi.yaml`
- WebSocket schema definitions: `contracts/websocket/events.ts`
- SQLC inputs: `db/platform/queries/*.sql`

Generation entrypoint:

```bash
make generate
```

Generated outputs include:
- `contracts/openapi/openapi.bundle.yaml`
- `contracts/generated/go/server_gen.go`
- `contracts/generated/typescript/api.ts`
- `internal/api/server_gen.go`
- `db/gen/go/platform/*`
- `contracts/websocket/events.schema.json`

## What Is Not Implemented Yet

These exist as plans, contracts, or scaffolding, but not as full runtime capabilities:
- durable execution engine in `engine/`
- live WebSocket runtime in `internal/ws`
- proxy capture runtime in `internal/proxy`
- replay execution runtime in `internal/replay`
- feature-complete TypeScript SDK

## Current Validation Surfaces

- `pnpm --filter web test`
- `pnpm --filter web test:e2e`
- `web/playwright.config.ts`
- `web/e2e/ui-smoke.spec.ts`

