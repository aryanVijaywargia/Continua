# Continua Shared Decisions

## Source-of-truth order
1. Live code in `cmd/`, `internal/`, `web/`, and `sdks/python/`
2. Contracts in `contracts/`
3. Platform schema + queries in `db/platform/`
4. `docs/PHASE5_CURRENT_STATE_REPORT.md` for the current architecture narrative
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
- Real SDK: `sdks/python`
- Stubbed or placeholder-heavy areas:
  - `engine/`
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

## Config reality
- Runtime config is env-only via `internal/config/config.go`.
- `config.example.yaml` is not the implementation contract and currently contains future-state drift.

## Useful reference docs
- `docs/PHASE5_CURRENT_STATE_REPORT.md`
- `docs/architecture/RULES.md`
- `docs/architecture/data-model.md`
