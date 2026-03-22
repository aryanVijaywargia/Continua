# Project Context

## Purpose

Continua is currently an **AI agent observability debugger**.

The implemented product path today is:

```text
Python SDK / custom client
  -> authenticated REST ingest
  -> Postgres persistence
  -> River background jobs
  -> REST read APIs
  -> embedded React debugger UI
```

Implemented product areas:
- trace, span, event, and session ingest
- durable idempotent batches plus true async ingest
- trace rollups and timeline read paths
- traces list, sessions list, and session detail pages
- failure-first trace detail workspace with payload inspection, state diff, and semantic events
- Python SDK support for traces, spans, sessions, batching, and async polling

Not yet implemented as product runtime:
- replay execution
- live WebSocket runtime
- proxy capture runtime
- score APIs
- TypeScript SDK parity
- durable engine runtime

## Tech Stack

### Backend
- **Go** - Primary server language
- **Uber Fx** - Dependency injection
- **pgx/v5** - PostgreSQL driver
- **River** - Background jobs in Postgres
- **sqlc** - Type-safe SQL code generation
- **golang-migrate** - Database migrations
- **Cobra** - CLI framework

### Frontend
- **Vite** - Build tool
- **React 18** - UI framework
- **TypeScript** - UI and generated API types
- **TanStack Query** - Data fetching/caching
- **Tailwind CSS** - Styling

### Database
- **PostgreSQL** - Primary database
- **SQLite** - Bootstrap scaffold only, not full runtime parity

### SDKs
- **Python SDK** - active SDK
- **TypeScript SDK** - stub package only

## Source Of Truth

Use these in order:
1. Live code in `cmd/`, `internal/`, `web/`, and `sdks/python/`
2. Contracts in `contracts/`
3. Platform schema and queries in `db/platform/`
4. `docs/DEBUGGER_PLATFORM_BASELINE.md`
5. `openspec/implemented/` and `openspec/changes/`

Important caveat: `openspec/specs/` is currently empty, so OpenSpec is not a complete current-state source on its own.

### Architecture Patterns

**Contract-First Development:**
1. OpenAPI spec (`contracts/openapi/openapi.yaml`) is source of truth for REST API
2. `contracts/websocket/events.ts` is the source of truth for the WebSocket schema, but not for an implemented live runtime
3. `make generate` regenerates all derived code
4. CI fails if generated code is out of sync

**Module Boundaries:**
```
cmd/continua/        → Server CLI entrypoint (Cobra)
contracts/           → API contracts (SOURCE OF TRUTH)
internal/            → Server internals (not importable externally)
pkg/                 → Shared public packages
engine/              → ISOLATED Go module (cannot import internal/)
db/platform/         → Migrations + sqlc queries
web/                 → Vite React SPA (embedded in Go binary)
sdks/                → TypeScript and Python SDKs
```

**Current runtime shape:**
- `internal/api` owns handlers, mapping, auth, and timeline helpers
- `internal/ingest` owns sync vs async orchestration plus shared write-path logic
- `internal/jobs` owns async ingest, rollups, and cleanup workers
- `internal/store` stays thin over sqlc plus selective handwritten SQL for dynamic search
- `web/src` is the active debugger frontend

**Current frontend shape:**
- `/traces` and `/sessions` are URL-driven list pages
- `/traces/:id` is a desktop/mobile debugger workspace
- running traces poll timeline events; there is no live WebSocket subscription

### Testing Strategy

- Go tests live beside the code and many integration-style tests use real Postgres helpers
- Frontend uses Vitest and Testing Library in `web/src`
- Python SDK uses pytest in `sdks/python/tests`
- Useful validation commands:
  - `go test ./internal/api/...`
  - `go test ./internal/ingest/...`
  - `go test ./internal/store/...`
  - `go test ./internal/jobs/...`
  - `pnpm --filter web test`
  - `cd sdks/python && uv run pytest`

## OpenSpec Conventions

- Use `openspec/changes/` for active or proposed work.
- Move completed changes into `openspec/implemented/`.
- Treat `openspec/implemented/` as history, not as the sole source of current runtime truth.
- When planning, prefer the live code plus `docs/DEBUGGER_PLATFORM_BASELINE.md` before older phase docs.

## Domain Context

**Core Concepts:**
- **Session**: ingest-linked grouping of traces with internal UUID plus `external_id`
- **Trace**: a single execution flow with internal UUID plus external `trace_id`
- **Span**: a unit of work with internal UUID plus external `span_id`
- **Event**: explicit span event stored in `span_events`
- **Timeline**: explicit events plus synthetic lifecycle markers derived from spans

## Important Constraints

- Generated code must never be manually edited
- Engine module cannot import from internal/ (Go enforced boundary)
- Web UI must be static (no SSR) for Go binary embedding
- All contract changes require `make generate` before commit
- Existing migrations are immutable
- Placeholder-heavy areas such as `internal/ws`, `internal/replay`, `internal/proxy`, and `engine/` should not be treated as implemented
