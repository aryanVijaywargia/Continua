# Architecture Overview

## Current Runtime

Continua's implemented platform is a single Go server plus an embedded React debugger UI.

The active request path today is:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> React debugger UI
```

## Runtime Components

### Platform Server

The Go server in `cmd/continua` is the real product runtime today. It provides:
- authenticated REST APIs from `contracts/openapi/openapi.yaml`
- ingest handling in `internal/api` + `internal/ingest`
- River worker startup in `internal/jobs`
- Postgres-backed store/query access in `internal/store`
- the embedded web UI from `internal/web`

### Web UI

The frontend in `web/` is a Vite React SPA built into `internal/web/static/` and embedded into the Go binary.

Implemented UI areas include:
- traces list with URL-driven filtering
- trace detail with failure-first triage
- trace detail workspace with tree rail, execution waterfall, and inspector tabs
- span tree and span detail surfaces
- payload inspection and truncation banners
- state diff and semantic state/decision events
- merged timeline view backed by polling
- sessions list and session detail views
- settings, auth recovery, command palette, and theming

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

### Trace Debugging

```text
GET /api/traces
GET /api/traces/{id}
GET /api/traces/{id}/spans
GET /api/traces/{id}/events
GET /api/sessions
GET /api/sessions/{id}
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
- sessions expose internal UUIDs plus `external_id`
- traces expose internal UUIDs plus external `trace_id`
- spans expose internal UUIDs plus external `span_id`
- span tree parent links use external `parent_span_id`

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

For the shortest current repo-verified handoff, see `docs/DEBUGGER_PLATFORM_BASELINE.md`.
