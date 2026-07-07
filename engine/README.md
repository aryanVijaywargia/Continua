# Continua Engine

The durable execution engine for AI agent workflows.

**Status:** Durable runtime implemented (preview). Executes Go-defined workflows end-to-end — activities, timers, signals, child workflows, cancellation, continue-as-new — with event-sourced history and crash-recovery replay across restarts. The public REST control plane is preview-gated and workflow authoring is Go-only.

## Scope

The engine module ships a working durable runtime:

- a dedicated Postgres `engine` schema with reversible migrations
- sqlc-backed query generation under `engine/db/gen/go/`
- an engine-local store with its own pgx connection pool and transaction support
- a workflow worker that claims runs, replays event-sourced history, and drives them to a terminal result
- an activity worker that claims tasks, runs handlers, and records completions/failures with retry backoff
- durable primitives: activities, timers, signals, child workflows, cancellation, and continue-as-new
- crash-recovery: runs and activities resume across process restarts via leases + history replay (see the restart suite in `cmd/continua-engine/runtime_e2e_test.go`)
- projection of run state into `public.traces` for the debugger's engine-runs console
- a `continua-engine` CLI with `version`, `migrate`, `serve`, `start`, `signal`, `cancel`, and `inspect`

What is still **preview / not yet there**:

- workflow authoring is Go-only — no TypeScript/Python authoring SDK
- no production path to register arbitrary user workflow definitions (the dark-launch runtime runs a fixed demo project)
- the public `/v1/engine/*` REST control plane is gated behind `X-Continua-Engine-Preview` + `ENGINE_PUBLIC_API_ENABLED`

## Activity Leases

Local activity workers heartbeat claimed tasks at half of the configured activity lease TTL while the handler is running. If the worker process crashes, the heartbeat stops, the lease expires, and the task becomes claimable by another worker.

Activity handlers must therefore be idempotent: activity execution is at-least-once across process crashes. If a running handler loses its lease, such as due to a slow heartbeat, its context is cancelled, but a reclaimed execution may briefly run concurrently until cancellation takes effect. After a genuine process crash, the prior execution is gone; a new worker resumes the task sequentially once the lease expires.

## Storage Model

`engine/` is an isolated Go module, but not a mandate for a separate physical database.

Current implemented guidance is:

- keep engine runtime state in the dedicated Postgres `engine` schema
- keep engine migrations, sqlc outputs, and runtime code isolated under `engine/`
- share the same physical Postgres deployment as the main `Continua` product by default
- use a separate engine connection pool so workflow/activity workers do not starve control-plane or debugger traffic
- avoid cross-schema foreign keys to `public.projects` in the foundation phase

### Definition Catalog

`engine.definition_catalog` reflects the definitions registered in the live engine runtime.
At `continua-engine serve` startup, the runtime rewrites the catalog by upserting the
registered definitions and deleting stale rows. The platform `StartRun` path validates
requests against this catalog, so inserting catalog rows out of band is unsupported; those
rows are removed at the next serve start. Richer liveness metadata is future work tracked
by the "definition catalog as registry truth" issue (#163).

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

# durable runtime (dark-launch demo project)
bin/continua-engine serve                                   # run workflow + activity + maintenance + projector workers
bin/continua-engine start  --instance-key <k> --definition <name> --version <v> --request-key <r> [--input <json>]
bin/continua-engine signal --instance-key <k> --signal-name <s> [--payload <json>]
bin/continua-engine cancel --instance-key <k>
bin/continua-engine inspect --instance-key <k>              # dump instance state + history as JSON
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
