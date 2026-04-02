# Backend Architecture

## Live server composition

`cmd/continua/main.go` starts an Fx app with:
- `internal/config`
- `internal/store`
- `internal/jobs`
- `internal/ingest`
- `internal/api`

The Go server is the current platform runtime. `engine/` is a separate module but is not part of the active product path.

Current frontend consumers of these backend reads include:
- the shared shell and overview route at `/`
- trace triage and detail
- session detail and session compare

## Request and job flow

### Protected REST request
1. Chi router in `internal/api/router.go`
2. API-key auth in `internal/api/middleware/auth.go`
3. feature handler in `internal/api/*_handlers.go`
4. store/service call
5. mapper in `internal/api/mapper.go`
6. JSON response

### Ingest flow
1. `POST /v1/ingest` in `internal/api/ingest_handlers.go`
2. project-scoped accept logic in `internal/ingest/service.go`
3. shared validation/write path in `internal/ingest/processor.go`
4. async batches and rollups through River workers in `internal/jobs`

### Session compare flow
1. `GET /api/sessions/{id}/compare` in `internal/api/sessions_handlers.go`
2. project scoping + request validation
3. compare store/query path over session traces, spans, and semantic events
4. mapper conversion into compare response types

## Active vs scaffolded packages

### Actively extended today
- `internal/api`
- `internal/ingest`
- `internal/jobs`
- `internal/store`
- `internal/config`
- `internal/web`

### Present but mostly placeholders
- `internal/proxy`
- `internal/ws`
- `internal/replay`
- `internal/alerts`
- `internal/export`
- `internal/state`
- `internal/telemetry`
- `engine/`

## Generated code path

| Source | Generated output |
|--------|------------------|
| `contracts/openapi/openapi.yaml` | `contracts/generated/go/server_gen.go` |
| `contracts/generated/go/server_gen.go` | `internal/api/server_gen.go` |
| `contracts/openapi/openapi.yaml` | `contracts/generated/typescript/api.ts` |
| `db/platform/queries/*.sql` | `db/gen/go/platform/*.go` |
| `contracts/websocket/events.ts` | `contracts/websocket/events.schema.json` |

Run `make generate` for all of the above.

## Config reality

Runtime config comes from `internal/config/config.go`, not from YAML files.

Current env vars include:
- `DATABASE_URL`
- `HOST`
- `PORT`
- `INGEST_TRUE_ASYNC_DEFAULT`
- `INGEST_DEPENDENCY_RETRY_WINDOW`
- `INGEST_FAILED_PAYLOAD_RETENTION`
- `RIVER_QUEUE_*`

`config.example.yaml` is future-facing and not a reliable implementation reference.

## Build outputs

- `make build-web` copies `web/dist/` into `internal/web/static/`
- `internal/web/embed.go` embeds `internal/web/static/`
- the Go server serves both API routes and the embedded SPA
