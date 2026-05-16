# Continua Shared Decisions

## Source-of-truth order
1. Live code in `cmd/`, `internal/`, `web/`, and `sdks/python/`
2. Contracts in `contracts/`
3. Platform schema + queries in `db/platform/`
4. Public docs in `docs-site/`
5. OpenSpec proposals and implemented changes for change history and planned direction

Important caveat: `openspec/specs/` is currently empty, so OpenSpec is not a complete current-state spec set.

## Contracts
- REST source of truth: `contracts/openapi/openapi.yaml`
- WebSocket schema source of truth: `contracts/websocket/events.ts`
- Current runtime uses the REST contract heavily.
- The WebSocket schema exists, but there is no fully implemented WebSocket runtime in `internal/ws` today.

## Code generation
- Preferred command: `make generate`
- Contract-only generation: `pnpm --filter @continua/contracts generate`
- Drift check: `scripts/check-generated.sh`

`make generate` currently does all of the following:
- bundles OpenAPI
- generates `contracts/generated/go/server_gen.go`
- generates `contracts/generated/typescript/api.ts`
- generates `contracts/websocket/events.schema.json`
- runs sqlc for `db/platform`
- runs sqlc for `engine/db`
- copies the generated Go server into `internal/api/server_gen.go`
- regenerates Python SDK types when the OpenAPI bundle exists

## Generated files: do not edit directly
- `contracts/openapi/openapi.bundle.yaml`
- `contracts/generated/go/server_gen.go`
- `contracts/generated/typescript/api.ts`
- `internal/api/server_gen.go`
- `db/gen/go/platform/*`
- `contracts/websocket/events.schema.json`
- `engine/db/gen/go/*`

## Current runtime architecture
- Fx app wiring lives in `cmd/continua/main.go`.
- Active backend layers:
  - `internal/api`
  - `internal/ingest`
  - `internal/jobs`
  - `internal/store`
  - `internal/config`
  - `internal/web`
- Active frontend/UI runtime: `web/src`
- Current user-facing frontend focus:
  - shared `AppShell` and `/` overview
  - traces list + sessions list URL state
  - session detail + session compare workspaces
  - failure-first trace detail workspace with desktop drawer and mobile four-tab composition
  - payload inspection + state diff + semantic reasoning surfaces
  - settings, auth recovery, command palette, theming
- Real SDK: `sdks/python`
- Engine module reality:
  - `engine/` now has schema/store/CLI foundation
  - runtime workflow/activity packages under `engine/internal/` are still placeholder-heavy
- Other stubbed or placeholder-heavy areas:
  - `internal/proxy`
  - `internal/ws`
  - `internal/replay`
  - `sdks/typescript`

## Data model semantics
- `projects` scope all protected data access.
- `ingest_batches` plus `ingest_batch_payloads` implement durable idempotency and true async ingest.
- `sessions` expose internal UUIDs plus user-facing `external_id`.
- `traces` expose internal UUIDs plus external `trace_id`.
- `spans` expose internal UUIDs plus external `span_id`; `parent_span_id` is also an external span ID.
- `span_events` store explicit events; the timeline API merges explicit events with synthetic span lifecycle markers.

## Database reality
- Postgres is the platform runtime source of truth.
- SQLite under `db/platform/migrations/sqlite/` is only an early bootstrap scaffold, not a full parity target.
- Engine DB work stays inside `engine/`.
- Engine foundation state lives in the dedicated Postgres `engine` schema.
- Engine migrations use their own migration bookkeeping table so they can coexist with platform migrations in the same physical database.
- Engine foundation does not add cross-schema foreign keys to `public.projects`.
- The repo is still pre-production, so platform and engine migrations may be rewritten or squashed until the first production release. After the first production release, migration history becomes immutable.

## Config reality
- Runtime config is env-only via `internal/config/config.go`.
- `config.example.yaml` is not the implementation contract and currently contains future-state drift.
- Engine config is env-only via `engine/internal/config/config.go`, with `ENGINE_DATABASE_URL` overriding `DATABASE_URL`.

## Engine foundation
- The engine store owns its own pgx pool with conservative defaults:
  - `MaxConns=10`
  - `MinConns=2`
  - `MaxConnLifetime=1h`
  - `MaxConnIdleTime=30m`
  - `HealthCheckPeriod=1m`
- Claimable engine tables use lease-based claiming with `claimed_by`, `claimed_at`, and `lease_expires_at`.
- Engine schema/query DDL uses fully-qualified `engine.*` names throughout.

## Engine maintenance ownership
- Engine maintenance in `engine/internal/worker/maintenance.go` owns due-timer wakeups for non-suspended runs and request-dedupe expiry.
- Activity retries use durable `available_at` scheduling on `engine.activity_tasks`; they do not introduce a separate maintenance loop.
- Root-side maintenance in `internal/jobs/` owns retention and bulk backfill triggering.

## Useful reference docs
- `docs-site/guides/installation.mdx`
- `docs-site/concepts/overview.mdx`
- `docs-site/concepts/data-model.mdx`
- `docs-site/concepts/events.mdx`
- `docs-site/concepts/ingest-lifecycle.mdx`

## Current validation surfaces
- `pnpm --filter web test`
- `pnpm --filter web test:e2e`
- `web/playwright.config.ts`
- `web/e2e/ui-smoke.spec.ts`
