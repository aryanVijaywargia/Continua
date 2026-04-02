# Continua Architecture Rules

> **Status: Current**
> These are the current anti-drift rules for the active product path.

## 10 Rules That Prevent Drift

### 1. Live runtime beats old docs

When docs disagree, prefer the current code in `cmd/`, `internal/`, `web/`, and `sdks/python/`.

### 2. ONE generation command

```bash
make generate
```

This is the repo-level generation entrypoint for OpenAPI outputs, sqlc, copied Go server types, and Python SDK types.

### 3. Contracts are source of truth

- `contracts/openapi/openapi.yaml` -> REST API
- `contracts/websocket/events.ts` -> WebSocket schema history

The runtime uses the REST contract heavily. The WebSocket schema exists, but there is no fully implemented WebSocket runtime today.

### 4. Generated files are tracked, not edited

Change the source inputs, not:
- `contracts/generated/`
- `internal/api/server_gen.go`
- `db/gen/go/platform/`
- `contracts/websocket/events.schema.json`

### 5. Postgres is the runtime database

`db/platform/migrations/postgres/` is the real platform schema. SQLite under `db/platform/migrations/sqlite/` is bootstrap-only scaffolding.

### 6. Runtime config is env-only

`internal/config/config.go` is the live config contract. `config.example.yaml` is future-state drift, not runtime truth.

### 7. Keep backend responsibilities split

- handlers in `internal/api/*_handlers.go`
- ingest orchestration in `internal/ingest`
- River work in `internal/jobs`
- thin persistence wrappers in `internal/store`
- DB rows mapped to API types in `internal/api/mapper.go`

### 8. Preserve current web state contracts

- list-page state belongs in URL params
- trace selection belongs in `?span=`
- session compare stays in `baseline_trace_id` / `candidate_trace_id`
- trace detail remains polling-based; do not assume live push

### 9. Future-looking directories are not product proof

Do not describe `engine/`, `internal/proxy`, `internal/ws`, `internal/replay`, `internal/alerts`, `internal/export`, `internal/state`, `internal/telemetry`, or `sdks/typescript` as implemented product surfaces unless the code in the current task makes that true.

### 10. Use the doc status labels consistently

- **Current** -> authoritative repo-verified guidance
- **Historical** -> preserved context, not the current architecture contract
- **Active change** -> proposal material under `openspec/changes/`, not current-state truth by itself
