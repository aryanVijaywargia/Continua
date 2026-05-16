<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

If `openspec/` is present in the working tree, open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

`openspec/` is gitignored — it's the internal product-development record, present only on the maintainer's local checkout. External contributors will not have it; fall back to checked-in code and [docs-site/](./docs-site/) as the authoritative sources.

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Repository Guidelines

## Current Repo Baseline
- Treat the checked-in code as the primary truth. Historical phase docs and some older architecture docs drift from the current implementation.
- The live product path today is: authenticated REST ingest -> Postgres persistence -> River background jobs -> REST read APIs -> embedded React debugger operator console.
- For current-state architecture, start with [`docs-site/concepts/`](./docs-site/concepts/) (overview, data-model, traces-spans-sessions, events, ingest-lifecycle).
- Use the checked-in code, contracts, and migrations as the authoritative current-state baseline.
- `docs/DEBUGGER_PLATFORM_BASELINE.md` and `docs/PHASE5_CURRENT_STATE_REPORT.md` are gitignored historical context. Use them only if present locally.
- If `openspec/` is present locally, it is useful for active proposals and archived work but not a complete source of current-state specs (`openspec/specs/` is empty).

## Documentation Status Convention
- `Current`: authoritative, repo-verified guidance for the current checkout.
- `Historical`: preserved context or archaeology; do not treat it as the current architecture contract.
- `Active change`: material under `openspec/changes/` if present locally; useful for intent/history, but not current-state truth by itself.

## Implemented Vs Scaffolded
- Active backend packages: `internal/api`, `internal/ingest`, `internal/jobs`, `internal/store`, `internal/config`, `internal/web`.
- Active frontend areas: `web/src/pages`, `web/src/components`, `web/src/utils`, `web/src/hooks`.
- Active SDK: `sdks/python`.
- Mostly scaffolded or placeholder today: `engine/`, `internal/proxy`, `internal/ws`, `internal/replay`, `internal/alerts`, `internal/export`, `internal/state`, `internal/telemetry`, `sdks/typescript`.
- Do not describe WebSockets, proxy capture, replay, framework adapters, or the durable engine as implemented unless you have added that code in the current task.

## Source Of Truth
- REST contract: `contracts/openapi/openapi.yaml`.
- WebSocket schema contract: `contracts/websocket/events.ts`.
- Platform DB schema truth: `db/platform/migrations/postgres/` and `db/platform/queries/`.
- Runtime behavior truth: `cmd/continua`, `internal/`, `web/`, `sdks/python/`.
- Shared agent reference: `.codex/references/decisions.md`.

## Project Structure
- `cmd/continua`: Cobra entrypoint for `serve`, `migrate`, and `version`.
- `internal/api`: OpenAPI-backed handlers, auth middleware, mappers, router, timeline helpers.
- `internal/ingest`: ingest request DTOs, validation, sync/async orchestration, shared write path.
- `internal/jobs`: River workers for async ingest, rollups, and payload cleanup.
- `internal/store`: sqlc-backed store wrappers plus custom trace search SQL.
- `db/platform`: Postgres migrations, SQLite bootstrap migration, SQLC query inputs.
- `contracts`: OpenAPI, WebSocket schemas, and generated client/server types.
- `web`: Vite React debugger UI.
- `sdks/python`: functional SDK with batching, trace/span/session helpers, and async ingest polling.
- `sdks/typescript`: early stub package, not a feature-complete SDK.
- `engine`: separate Go module reserved for future durable execution work.

## Architecture Conventions
- Keep handlers feature-split in `internal/api/`:
  - `ingest_handlers.go`
  - `traces_handlers.go`
  - `sessions_handlers.go`
  - `timeline.go`
  - shared helpers in `server_helpers.go` and mapping in `mapper.go`
- Keep ingest orchestration in `internal/ingest/service.go`.
- Keep shared ingest validation and DB write-path logic in `internal/ingest/processor.go`.
- Keep background concerns in `internal/jobs/`, not in handlers.
- Keep store wrappers thin. Use sqlc queries first; use handwritten SQL only where dynamic filtering genuinely requires it, as in `internal/store/search.go`.
- Never leak sqlc/model types directly to API responses. Always map through `internal/api/mapper.go`.
- Project scoping is enforced through `internal/api/middleware/auth.go`; protected handlers should pull `project_id` from request context, not from request bodies.

## Contracts, Generation, And Generated Files
- Run `make generate` after changing:
  - `contracts/openapi/openapi.yaml`
  - `contracts/websocket/events.ts`
  - `db/platform/queries/*.sql`
  - `db/platform/sqlc.yaml`
  - platform migrations when they affect generated types
- Generated outputs that must not be edited directly:
  - `contracts/openapi/openapi.bundle.yaml`
  - `contracts/generated/go/server_gen.go`
  - `contracts/generated/typescript/api.ts`
  - `internal/api/server_gen.go`
  - `db/gen/go/platform/*`
  - `contracts/websocket/events.schema.json`
- `make generate` copies the generated Go server types from `contracts/generated/go/server_gen.go` into `internal/api/server_gen.go`.

## Database And Migration Rules
- Postgres is the real platform runtime database.
- SQLite exists only as an early bootstrap scaffold under `db/platform/migrations/sqlite/`; do not assume full parity with Postgres behavior.
- This repo is still pre-production. Until the first production release, migrations under `db/platform/migrations/` and `engine/db/migrations/` may be rewritten, renumbered, or squashed if that keeps the pre-release schema history cleaner.
- After the first production release, treat existing migrations under `db/platform/migrations/` and `engine/db/migrations/` as immutable.
- If you rewrite a pre-production migration, update any dependent down migrations, migration smoke tests, generated code, and docs that reference the old numbering or behavior.
- Create new migrations with `make migrate-create name=<description>`.
- Current important platform tables include `projects`, `ingest_batches`, `ingest_batch_payloads`, `sessions`, `traces`, `spans`, and `span_events`.

## Config Reality
- The live server config is env-only via `internal/config/config.go`.
- Required env var: `DATABASE_URL`.
- Important optional env vars include `HOST`, `PORT`, `INGEST_TRUE_ASYNC_DEFAULT`, `INGEST_DEPENDENCY_RETRY_WINDOW`, `INGEST_FAILED_PAYLOAD_RETENTION`, and the `RIVER_QUEUE_*` worker counts.
- `config.example.yaml` is not the runtime config source and currently describes capabilities the server does not fully implement. Do not use it as the implementation contract.

## Frontend Reality
- The web UI is a Vite SPA embedded into the Go binary through `internal/web/static`.
- Current routes are `/`, `/traces`, `/traces/:id`, `/sessions`, `/sessions/:id`, `/sessions/:id/compare`, and `/settings`.
- The app uses a shared `AppShell` with primary navigation, route-aware shell chrome, API-key status, theme controls, and command palette access.
- The overview route is frontend-only and derives its snapshot from existing trace and session list endpoints.
- The traces page is URL-driven and exposes current backend filters.
- Trace detail is failure-first and uses polling of `/api/traces/{id}/events`, not a live WebSocket subscription.
- Desktop trace detail uses a drawer for trace context; mobile trace detail uses `Summary`, `Execution`, `Timeline`, and `State` top-level tabs.
- Session detail and session compare preserve URL-driven state and return navigation.
- Payload inspection, truncation banners, breadcrumb navigation, local tree-rail quick filters, and session drill-down are implemented in `web/src`.

## Testing Expectations
- Add tests for new behavior.
- Go tests live next to code. Integration-style tests commonly use a real Postgres pool via `internal/testutil`.
- Useful targeted suites:
  - `go test ./internal/api/...`
  - `go test ./internal/ingest/...`
  - `go test ./internal/store/...`
  - `go test ./internal/jobs/...`
  - `pnpm --filter web test`
  - `pnpm --filter web test:e2e`
  - `cd sdks/python && uv run pytest`
- Full validation:
  - `make generate`
  - `make lint`
  - `make test`
- `make test-integration` exists for `-tags=integration`, but many current backend tests already use real DB access without that tag. Read the package tests before assuming a suite is purely unit-level.

## OpenSpec Expectations
- OpenSpec (`openspec/`) is gitignored and only present on the maintainer's local checkout. If not present, skip this section.
- If present, use OpenSpec for new capabilities, breaking changes, architectural shifts, or major performance/security work.
- For implementation against an existing change:
  - read `proposal.md`
  - read `design.md` if present
  - read `tasks.md`
  - implement in task order
- Because `openspec/specs/` is empty, do not assume OpenSpec alone describes the current repo state. Cross-check with code, contracts, migrations, and [`docs-site/concepts/`](./docs-site/concepts/).

## Project-local Codex Context
- Repo-local Codex assets live in `.codex/`. Prefer these over `.claude/` when working from Codex.

### Repo-local skills
- `continua-backend-dev`: current Go platform server, REST/API, sqlc/store, migrations, and River-backed backend workflows.
- `continua-debugger-ui`: current React debugger UI, app shell, overview, trace workspace, session compare, URL-state patterns, payload inspection, settings, command palette, and theming.
- `continua-observability`: trace/span/session/event model, async ingest lifecycle, rollups, timeline semantics, and debugger data surfaces.
- `continua-integrations`: Python SDK, contract-driven SDK generation, TypeScript SDK stub status, and integration-boundary planning.
- `continua-testing`: suite selection, real-DB test patterns, web Vitest coverage, Playwright smoke coverage, and SDK verification.

### How to use repo-local skills
- Open the matching `.codex/skills/<skill>/SKILL.md` when the task fits the skill.
- Load linked `resources/` or `references/` files only when needed.
- Start with `.codex/references/decisions.md` for shared repo rules and current-state boundaries.
- Prefer `continua-debugger-ui` for `web/` product work; use `continua-observability` alongside it when the task changes trace/session/event semantics rather than just UI behavior.

### Codex guardrails
- Do not edit generated files directly; change the source inputs and run `make generate`.
- Follow the repo's pre-production migration policy: migration rewrites are allowed until the first production release, but become immutable after that point.
- Avoid direct `.env*` reads or writes.
- Avoid broad staging commands like `git add .`, `git add -A`, or wildcard staging.
- Format edited Go files with `gofmt` and `goimports` when available.
- See `.codex/references/guardrails.md`, `.codex/references/commands.md`, and `.codex/references/subagents.md` for supporting workflow notes.
