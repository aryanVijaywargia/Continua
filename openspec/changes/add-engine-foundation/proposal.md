# Change: Add Engine Foundation (Phase 10.1)

## Why

The `engine/` module is fully scaffolded but contains no real code: a placeholder `main.go`, an empty migration, no queries, and no store layer. Future engine phases (runtime, workers, activity execution) all depend on a working schema, store, CLI, and generation pipeline. This change establishes that foundation without touching the live product path.

## What Changes

### Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| **engine-schema-foundation** | NEW | Dedicated `engine` schema in Postgres with 7 core tables, enums, indexes, and reversible migrations |
| **engine-store-foundation** | NEW | Engine-local store package with its own connection pool, transaction support, sqlc-backed wrappers, and sentinel errors |
| **engine-cli-foundation** | NEW | Cobra-based `continua-engine` binary with `version`, `migrate up`, and `migrate down [steps]` commands |
| **engine-generation-pipeline** | NEW | Makefile, `scripts/generate.sh`, `scripts/check-generated.sh`, and CI generate-check updated to always run engine sqlc generation |

### Key Design Decisions

1. **Dedicated `engine` schema**: Engine tables live in `engine.*`, not `public.engine_*`. This gives clean namespace isolation without requiring a separate physical database.
2. **Shared Postgres, separate pool**: Engine shares the physical database but uses its own connection pool with conservative defaults (`MaxConns=10`) so engine work cannot starve debugger/API traffic.
3. **No cross-schema FKs**: `project_id` is kept on engine tables for scoping but has no FK to `public.projects` in this phase. Engine-internal FKs use `RESTRICT` (no `CASCADE`).
4. **Full store coverage for all tables**: All seven tables — including `inbox`, `activity_tasks`, `request_dedupe`, and `projection_checkpoints` — get schema, sqlc queries, store wrappers, and tests in this phase. This establishes verified claim/lease/checkpoint patterns that runtime phases can build on without reworking the store layer.
5. **Intentional naming distinctions**: `runs.ready_at` vs `inbox.available_at`/`activity_tasks.available_at` — different semantics, different column names.
6. **Platform-first test setup**: Engine test helpers apply platform migrations before engine migrations in every test database, making shared-DB coexistence the default rather than a special case.

### Breaking Changes

None. The live product path (`POST /v1/ingest`, `/api/traces*`, `/api/sessions*`, debugger UI, `cmd/continua` runtime) is completely untouched.

## Impact

### Affected Specs

- `engine-schema-foundation` (new)
- `engine-store-foundation` (new)
- `engine-cli-foundation` (new)
- `engine-generation-pipeline` (new)

### Affected Code

| Path | Change |
|------|--------|
| `engine/db/migrations/postgres/000001_engine_foundation.up.sql` | ADD: engine schema, enums, 7 tables, indexes, FKs |
| `engine/db/migrations/postgres/000001_engine_foundation.down.sql` | ADD: reverse-order drops without CASCADE |
| `engine/db/queries/*.sql` | ADD: sqlc query files for all 7 tables plus claim/lease/checkpoint operations |
| `engine/db/sqlc.yaml` | MODIFY: ensure schema path includes new migration |
| `engine/cmd/continua-engine/main.go` | REPLACE: placeholder with Cobra root + version/migrate commands |
| `engine/internal/config/config.go` | ADD: env-only config with `ENGINE_DATABASE_URL` fallback to `DATABASE_URL` |
| `engine/internal/store/store.go` | ADD: pool, `Store`, `Tx`, `BeginTx`, sentinel errors, sqlc wrappers |
| `engine/internal/store/*_test.go` | ADD: transaction wiring, constraint, lease, history, checkpoint tests |
| `engine/go.mod` | MODIFY: add Cobra dependency |
| `Makefile` | MODIFY: ensure `generate` target always runs engine sqlc |
| `scripts/generate.sh` | MODIFY: unconditionally run engine sqlc generation |
| `scripts/check-generated.sh` | No change needed (already runs `make generate`) |
| `engine/README.md` | MODIFY: update status from pre-scaffolded to schema/store foundation |
| `docs/DEBUGGER_PLATFORM_BASELINE.md` | MODIFY: note engine schema foundation exists |

### Not Affected

- `cmd/continua` — no changes to the main server binary
- `internal/api`, `internal/ingest`, `internal/jobs`, `internal/store` — untouched
- `contracts/openapi/openapi.yaml` — no API surface changes
- `web/src` — no frontend changes
- `sdks/python`, `sdks/typescript` — no SDK changes
- `db/platform` — no platform schema or query changes

## Assumptions

- Shared physical Postgres remains the default deployment model.
- Engine isolation uses a dedicated `engine` schema, not `public.engine_*` prefixed tables.
- `definition_version` is `TEXT` (not a versioned integer or semver).
- No public execution APIs, debugger engine UI, Python control client, subworkflows, sticky cache, WebSocket push, or multi-language execution are included in Phase 10.1.
