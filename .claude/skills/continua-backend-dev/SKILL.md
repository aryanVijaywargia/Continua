---
name: continua-backend-dev
description: Backend development guide for Continua's current Go platform server. Use when changing REST handlers, ingest flows, River jobs, store/query code, migrations, or contract-driven backend behavior in cmd/, internal/, db/, or contracts/.
---

# Continua Backend Development

## Use this skill when
- editing Go code under `cmd/continua`, `internal/api`, `internal/ingest`, `internal/jobs`, or `internal/store`
- changing platform SQL under `db/platform`
- changing OpenAPI-backed backend behavior in `contracts/openapi/openapi.yaml`
- adding migrations, store methods, handlers, or River worker behavior

## Read first
- [../../references/decisions.md](../../references/decisions.md)

## Current backend shape
- `cmd/continua/main.go` wires the Fx app.
- `internal/api` is feature-split and owns auth, handler wiring, mapping, and timeline pagination.
- `internal/ingest/service.go` owns sync vs async accept/orchestrate behavior.
- `internal/ingest/processor.go` owns validation and the shared trace/span/event write path.
- `internal/jobs` owns River workers for async ingest, rollups, and cleanup.
- `internal/store` is thin over sqlc plus one handwritten dynamic query path in `search.go`.

## Quick workflows

### REST/API change
- Edit `contracts/openapi/openapi.yaml`
- Run `make generate`
- Implement or update handlers in `internal/api/`
- Add or update store/query code if required
- Map DB rows to API types in `internal/api/mapper.go`
- Add tests in the touched backend package

### Store/query change
- Edit `db/platform/queries/*.sql`
- Add a migration if schema changes
- Run `make generate`
- Expose a thin method from `internal/store`
- Use the store from handlers/services/jobs

### Ingest change
- Keep request/response shape handling in `internal/api/ingest_handlers.go`
- Keep sync/async branching in `internal/ingest/service.go`
- Keep shared validation/write-path logic in `internal/ingest/processor.go`
- Keep queue behavior in `internal/jobs`

## Current file layout conventions

### `internal/api`
- `server.go`: `Server` struct, constructor, shared constants
- `router.go`: top-level router assembly
- `server_helpers.go`: shared response helpers and request normalization
- `middleware/auth.go`: API-key auth and project scoping
- `ingest_handlers.go`, `traces_handlers.go`, `sessions_handlers.go`: feature handlers
- `timeline.go`: cursor pagination and synthetic-event ordering
- `mapper.go`: DB -> API mapping only

### `internal/store`
- prefer store wrapper methods over using generated queries directly outside store code
- keep wrappers thin and named after the business operation
- use handwritten SQL only when sqlc is a poor fit for dynamic filters, as in `search.go`

## Backend guardrails
- Always map platform models to API types.
- Do not bypass project scoping; protected handlers should get `project_id` from auth middleware context.
- Do not put SQL in handlers.
- Do not collapse feature-split handler files back into a generic `handlers.go`.
- Do not move processor logic into API handlers or River workers.
- Treat Postgres as the runtime DB. SQLite is not a full parity target today.
- Runtime config is env-only in `internal/config/config.go`; do not design around `config.example.yaml`.

## Useful references
- [architecture.md](resources/architecture.md)
- [api-patterns.md](resources/api-patterns.md)
- [database.md](resources/database.md)
- [testing.md](resources/testing.md)
