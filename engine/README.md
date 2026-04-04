# Continua Engine

The durable execution engine for AI agent workflows.

**Status:** Schema/store foundation implemented. Runtime workflow execution is still future work.

## Foundation Scope

The engine module now ships the Phase 10.1 foundation:

- a dedicated Postgres `engine` schema with reversible migrations
- sqlc-backed query generation under `engine/db/gen/go/`
- an engine-local store with its own pgx connection pool and transaction support
- a `continua-engine` CLI with `version`, `migrate up`, and `migrate down <steps>`

What this does **not** include yet:

- workflow execution
- history replay
- activity workers
- public execution APIs
- debugger UI for engine state

## Storage Model

`engine/` is an isolated Go module, but not a mandate for a separate physical database.

Current implemented guidance is:

- keep engine runtime state in the dedicated Postgres `engine` schema
- keep engine migrations, sqlc outputs, and runtime code isolated under `engine/`
- share the same physical Postgres deployment as the main `Continua` product by default
- use a separate engine connection pool so workflow/activity workers do not starve control-plane or debugger traffic
- avoid cross-schema foreign keys to `public.projects` in the foundation phase

## Module Isolation

This is a fully isolated Go module with its own internal packages and generated DB layer.

| Can Do | Cannot Do |
|--------|-----------|
| ✅ Import own `internal/*` | ❌ Import root's `internal/*` |
| ✅ Import own `db/gen/go/*` | ❌ Import root's `db/gen/go/*` |

## Building

```bash
make build-engine
```

## CLI

The engine CLI is built as `bin/continua-engine`.

```bash
bin/continua-engine version
bin/continua-engine migrate up
bin/continua-engine migrate down 1
```

Database config is env-only:

- `ENGINE_DATABASE_URL` overrides `DATABASE_URL`
- `DATABASE_URL` is the fallback when no engine-specific URL is set

## Schema Overview

The foundation migration creates these engine tables:

- `engine.instances`
- `engine.runs`
- `engine.history`
- `engine.inbox`
- `engine.activity_tasks`
- `engine.request_dedupe`
- `engine.projection_checkpoints`

It also defines engine-local enums for instance lifecycle, run lifecycle, activity task status, inbox status, and request dedupe status.
