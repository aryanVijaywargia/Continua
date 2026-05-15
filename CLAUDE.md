<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

If `openspec/` is present in the working tree, open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

`openspec/` is gitignored — it's the internal product-development record, present only on the maintainer's local checkout. External contributors will not have it; in that case, fall back to checked-in code, [docs-site/](./docs-site/), and [docs/architecture/](./docs/architecture/) as the authoritative sources.

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current Baseline

Continua is currently a Go/React monorepo whose real product path is:

1. authenticated ingest over REST
2. Postgres-backed storage
3. River workers for async ingest, rollups, and cleanup
4. React debugger UI for traces, sessions, span trees, payload inspection, and merged timelines

Do not assume replay, WebSocket runtime, proxy capture, durable engine workflows, or a real TypeScript SDK are already implemented. Most of those areas are scaffolded only.

Start discovery from:
- `AGENTS.md`
- `docs-site/concepts/overview.mdx` (current architecture)
- `docs/architecture/overview.md`
- `.claude/skills/references/decisions.md` (if present locally)
- the live package you are editing

Use the checked-in code, contracts, and migrations as the authoritative current-state baseline. `docs/DEBUGGER_PLATFORM_BASELINE.md` and `docs/PHASE5_CURRENT_STATE_REPORT.md` are gitignored historical context — use them only if present locally; do not rely on them in external clones.

## What Exists Today

### Active platform areas
- `cmd/continua`: Cobra commands for `serve`, `migrate`, `version`
- `internal/api`: auth middleware, handlers, mappers, timeline pagination
- `internal/ingest`: sync/async ingest orchestration and shared write path
- `internal/jobs`: River workers
- `internal/store`: sqlc-backed store wrappers and trace search
- `web/src`: traces list, sessions list/detail, trace detail, payload inspector, failure-first UI
- `sdks/python`: real SDK with batching, trace/span/session helpers, async ingest polling

### Mostly scaffolded
- `engine/`
- `internal/proxy`
- `internal/ws`
- `internal/replay`
- `sdks/typescript`

## Discovery Rules

- Treat checked-in code as the primary truth.
- Use [docs-site/concepts/](./docs-site/concepts/) and [docs/architecture/](./docs/architecture/) to understand repo reality quickly.
- If `openspec/` is present locally, use it for proposals and active change context (note: `openspec/specs/` is currently empty).
- If a doc conflicts with code, trust the code and update the doc if it is part of the task.

## Claude Skill Map

- `continua-backend-dev`: backend, REST/API, store, migrations, River jobs
- `continua-debugger-ui`: debugger frontend, trace workspace, traces/sessions pages, settings, theming
- `continua-observability`: trace/span/session/event semantics, ingest lifecycle, rollups, timeline semantics
- `continua-integrations`: Python SDK, TypeScript SDK stub, contract-driven SDK generation, proxy boundary
- `continua-testing`: test selection and verification strategy

## Core Architecture

### Backend structure
- `cmd/continua/main.go` wires Fx modules from `internal/config`, `internal/store`, `internal/jobs`, `internal/ingest`, and `internal/api`.
- `internal/api/router.go` mounts:
  - public `GET /api/health`
  - API-key-protected OpenAPI handlers
  - embedded SPA fallback
- `internal/api/middleware/auth.go` resolves `project_id` from the API key and injects it into request context.
- `internal/ingest/service.go` owns sync vs true-async acceptance.
- `internal/ingest/processor.go` owns shared validation and DB writes.
- `internal/jobs` owns the River workers:
  - ingest batch worker
  - trace rollup worker
  - payload cleanup worker
- `internal/store` is thin over sqlc, except `search.go`, which builds dynamic SQL for trace filters.

### Frontend structure
- `web/src/pages/TracesPage.tsx`: URL-driven trace filters and pagination
- `web/src/pages/TraceDetailPage.tsx`: failure-first debugger layout
- `web/src/pages/useTraceTimeline.ts`: paginated timeline bootstrap + polling
- `web/src/components/PayloadInspector.tsx`: interactive JSON viewer
- `web/src/components/Timeline.tsx`: merged explicit/synthetic events
- `web/src/components/SpanTree.tsx` and `SpanDetail.tsx`: tree navigation and payload/context details

### Data model reality
- `projects` gate all protected access.
- `ingest_batches` and `ingest_batch_payloads` support durable idempotency and true async ingest.
- `sessions` use internal UUIDs plus `external_id`.
- `traces` use internal UUIDs plus external `trace_id`.
- `spans` use internal UUIDs plus external `span_id`; tree links use external `parent_span_id`.
- `span_events` store explicit events; the timeline API merges them with synthetic span lifecycle markers.

## Contracts And Generation

### Sources of truth
- REST: `contracts/openapi/openapi.yaml`
- WebSocket schema: `contracts/websocket/events.ts`
- SQLC inputs: `db/platform/queries/*.sql`

### Required generation step
Run `make generate` after changing:
- OpenAPI
- WebSocket event schemas
- SQLC queries or config
- migrations that affect generated code expectations

### Generated files
Never hand-edit:
- `contracts/openapi/openapi.bundle.yaml`
- `contracts/generated/go/server_gen.go`
- `contracts/generated/typescript/api.ts`
- `internal/api/server_gen.go`
- `db/gen/go/platform/*`
- `contracts/websocket/events.schema.json`

Note: `make generate` copies the generated Go server code into `internal/api/server_gen.go`.

## Current API Surface

OpenAPI currently defines:
- `POST /v1/ingest`
- `GET /v1/ingest/batches/{id}`
- `GET /api/traces`
- `GET /api/traces/{id}`
- `GET /api/traces/{id}/spans`
- `GET /api/traces/{id}/events`
- `GET /api/sessions`
- `GET /api/sessions/{id}`

If a task assumes trace create/update endpoints, score CRUD, WebSocket endpoints, or replay endpoints already exist, verify first. They do not today.

## Config Reality

- Live config is env-only through `internal/config/config.go`.
- Required: `DATABASE_URL`
- Common optional vars: `HOST`, `PORT`, `INGEST_TRUE_ASYNC_DEFAULT`, `INGEST_DEPENDENCY_RETRY_WINDOW`, `INGEST_FAILED_PAYLOAD_RETENTION`, `RIVER_QUEUE_*`
- `config.example.yaml` is not the runtime source of truth and contains future-state drift.

## Database Rules

- Postgres is the actual runtime DB.
- SQLite under `db/platform/migrations/sqlite/` is an early bootstrap scaffold, not full parity.
- Existing migrations are immutable.
- Create new migrations with `make migrate-create name=<description>`.

## Workflow Patterns

### Backend API change
1. Edit `contracts/openapi/openapi.yaml`
2. Run `make generate`
3. Implement/adjust handlers under `internal/api`
4. Add or update store/query code if needed
5. Map DB types to API types in `mapper.go`
6. Add tests in the touched package

### DB/query change
1. Edit or add SQL under `db/platform/queries/`
2. Add migration if schema changes
3. Run `make generate`
4. Wire store methods under `internal/store`
5. Update handlers/services/jobs
6. Add DB-backed tests

### Ingest or async job change
1. Keep handler changes in `internal/api/ingest_handlers.go`
2. Keep acceptance/orchestration in `internal/ingest/service.go`
3. Keep validation/write-path logic in `internal/ingest/processor.go`
4. Keep queue behavior in `internal/jobs`

### Web change
1. Prefer typed API access through `web/src/api/client.ts`
2. Preserve URL-driven state patterns already used on traces and trace detail
3. Remember the timeline is polling-based today, not WebSocket-driven
4. Add Vitest coverage for new UI logic when practical

### Python SDK change
1. Check whether the change is contract-driven or SDK-only
2. If contract-driven, update OpenAPI first and run `make generate`
3. Keep batching/retry behavior in `client.py` and `batch.py`
4. Keep context logic in `trace.py`, `span.py`, `session.py`
5. Run `cd sdks/python && uv run pytest`

### TypeScript SDK change
- Treat `sdks/typescript` as a stub package unless the task is explicitly to build it out.
- A real TS SDK expansion is substantial enough that it should usually start with OpenSpec.

## Testing

Useful commands:

```bash
make dev
make dev-server
make dev-web
make generate
make lint
make test
go test ./internal/api/...
go test ./internal/ingest/...
go test ./internal/store/...
go test ./internal/jobs/...
pnpm --filter web test
cd sdks/python && uv run pytest
```

Notes:
- Many backend tests use a real Postgres database through `internal/testutil`, even without the `integration` build tag.
- Do not assume a package has clean unit-test isolation; read the tests first.

## Review And Safety

- Never fix failures by deleting tests or bypassing checks.
- Never edit generated files directly.
- Never assume placeholder packages are a supported extension point just because the directory exists.
- Prefer updating docs and skill guidance when you discover repo drift instead of encoding the wrong assumption into new code.
