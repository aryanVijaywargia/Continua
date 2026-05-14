## 1. Migration & Schema

- [x] 1.1 Keep `engine/db/migrations/postgres/0000_placeholder.sql` as-is (existing migrations are immutable). Create `000001_engine_foundation.up.sql` with: `CREATE SCHEMA IF NOT EXISTS engine`, five enums, seven tables with all columns/defaults/FKs, and all indexes (unique, composite, partial claim indexes)
- [x] 1.2 Create `000001_engine_foundation.down.sql` dropping objects in reverse FK order: `projection_checkpoints`, `request_dedupe`, `activity_tasks`, `inbox`, `history`, `runs`, `instances`, then enums, then `DROP SCHEMA engine`
- [x] 1.3 Verify golang-migrate ignores the placeholder (it lacks `.up.sql`/`.down.sql` suffixes) and only picks up `000001_*`

**Validation:** Migration files parse as valid SQL; `embed.go` compiles

## 2. SQLC Query Files

- [x] 2.1 Create `engine/db/queries/instances.sql` with create, get by ID, get by project+key, list by project, update status queries (all using `engine.instances`)
- [x] 2.2 Create `engine/db/queries/runs.sql` with create, get by ID, list by instance, update status, claim next eligible run (dual-condition: fresh `queued` OR `running` with expired lease, `FOR UPDATE SKIP LOCKED`) queries
- [x] 2.3 Create `engine/db/queries/history.sql` with append event, get by run (ordered by `sequence_no`), get by instance (ordered by `id`) queries
- [x] 2.4 Create `engine/db/queries/inbox.sql` with create, claim next (dual-condition: fresh `pending` OR `claimed` with expired lease, `SKIP LOCKED`), mark processed, mark discarded queries
- [x] 2.5 Create `engine/db/queries/activity_tasks.sql` with create, get by run+key, claim next (dual-condition: fresh `queued` OR `claimed` with expired lease, `SKIP LOCKED`), complete, fail queries
- [x] 2.6 Create `engine/db/queries/request_dedupe.sql` with create, get by scope+key, finalize with response, finalize with error queries
- [x] 2.7 Create `engine/db/queries/projection_checkpoints.sql` with get checkpoint and advance checkpoint (must not move backward) queries

**Validation:** All query files use fully-qualified `engine.*` table names

## 3. Generation Pipeline

- [x] 3.1 Add engine sqlc generation step to the `generate` target in `Makefile` (currently only runs platform sqlc — `Makefile:35`). Add `cd engine/db && sqlc generate` after the platform step
- [x] 3.2 Update `scripts/generate.sh` to run engine sqlc unconditionally (remove the conditional check for non-empty queries)
- [x] 3.3 Verify `engine/db/sqlc.yaml` schema path correctly picks up the new migration file
- [x] 3.4 Run `make generate` and confirm engine sqlc output appears in `engine/db/gen/go/`
- [x] 3.5 Verify `scripts/check-generated.sh` catches engine drift (it runs `make generate`, so inherits the Makefile fix)

**Validation:** `make generate` succeeds with no drift; generated Go types include all engine enums and table models
Note: in this already-dirty worktree, idempotency was verified by comparing `git status --porcelain` before and after a repeated `make generate` run; the status output was unchanged.

## 4. Engine Config

- [x] 4.1 Create `engine/internal/config/config.go` with env-only loading: `ENGINE_DATABASE_URL` overrides `DATABASE_URL`, clear error when neither is set
- [x] 4.2 Add pool defaults: `MaxConns=10`, `MinConns=2`, `MaxConnLifetime=1h`, `MaxConnIdleTime=30m`, `HealthCheckPeriod=1m`

**Validation:** Config loads from env vars; missing DB URL returns a clear error

## 5. Engine Store

- [x] 5.1 Create `engine/internal/store/store.go` with `Store` struct, pgx pool initialization using config defaults, `Close()`, `BeginTx()`, and `Tx` type
- [x] 5.2 Add sentinel errors: `ErrNotFound`, `ErrAlreadyExists`, `ErrStaleCheckpoint`
- [x] 5.3 Add thin sqlc-backed wrapper methods for instances, runs, history (named after business operations)
- [x] 5.4 Add thin sqlc-backed wrapper methods for inbox, activity_tasks, request_dedupe, projection_checkpoints
- [x] 5.5 Add claim/lease wrapper methods for runs, inbox, and activity_tasks

**Validation:** Store compiles; methods match sqlc-generated signatures

## 6. Engine CLI

- [x] 6.1 Replace `engine/cmd/continua-engine/main.go` with Cobra root command + `version` subcommand
- [x] 6.2 Add `migrate up` subcommand using golang-migrate directly with embedded migration files and DB URL from config (same pattern as `cmd/continua/main.go:86-118`, not through the store layer)
- [x] 6.3 Add `migrate down [steps]` subcommand with required step count (no accidental full rollback; see design.md Decision 10 for rationale vs root CLI behavior)
- [x] 6.4 Add `cobra` to `engine/go.mod` and run `go mod tidy`
- [x] 6.5 Verify `make build-engine` produces a working `bin/continua-engine` binary

**Validation:** `continua-engine version`, `continua-engine migrate up`, and `continua-engine migrate down 1` all work

## 7. Test Harness

- [x] 7.1 Create engine-local test helper that provisions a test database, locates the repo root via `runtime.Caller` (cwd-independent), applies all platform migrations first (from `<repo-root>/db/platform/migrations/postgres/`), then engine migrations (do not import root `internal/testutil`)
- [x] 7.2 Use `ENGINE_TEST_DATABASE_URL` with fallback to `TEST_DATABASE_URL`, then a hardcoded default test URL. Do NOT use `DATABASE_URL` — engine tests run destructive operations (down migrations, schema drops)
- [x] 7.3 Document PG 13+ assumption (required for `gen_random_uuid()` in platform migrations)

**Validation:** `cd engine && go test ./...` connects to Postgres and applies both migration sets

## 8. Store & Schema Tests

- [x] 8.1 Add transaction wiring test: begin→write→rollback→verify absent; begin→write→commit→verify present
- [x] 8.2 Add constraint tests for `instance_key`, `run_number`, `activity_key`, `request_scope + request_key`, and inbox dedupe uniqueness
- [x] 8.3 Add lease tests for runs: claim succeeds for eligible rows, active lease blocks duplicate claim, expired lease is reclaimable
- [x] 8.4 Add lease tests for activity_tasks: same claim/block/reclaim pattern
- [x] 8.5 Add lease tests for inbox: same claim/block/reclaim pattern
- [x] 8.6 Add history tests: monotonic `id` allocation order under sequential inserts, stable per-run `sequence_no` ordering
- [x] 8.7 Add checkpoint tests: advance succeeds, backward advance returns `ErrStaleCheckpoint`
- [x] 8.8 Add migration smoke tests: `continua-engine migrate up` then `continua-engine migrate down 1` round-trips cleanly

**Validation:** `cd engine && go test ./...` passes all tests

## 9. Regression Guard

- [x] 9.1 Run existing platform test suites: `go test ./internal/api/... ./internal/ingest/... ./internal/store/... ./internal/jobs/...`
- [x] 9.2 Verify `make generate` produces no drift (engine + platform)
- [x] 9.3 Verify `make build` (server + web) still succeeds

**Validation:** All existing tests pass; no regressions in the live product path

## 10. Documentation

- [x] 10.1 Update `engine/README.md`: status from "pre-scaffolded" to "schema/store foundation"; document `continua-engine` CLI commands and schema overview
- [x] 10.2 Update `docs/DEBUGGER_PLATFORM_BASELINE.md`: add note that engine schema foundation exists, runtime still not implemented
- [x] 10.3 Update `.codex/references/decisions.md`: add engine schema/store decisions

**Validation:** Docs accurately reflect the new baseline

---

### Parallelization Notes

- Tasks 1 and 4 can run in parallel (migrations and config are independent)
- Task 2 depends on task 1 (queries reference migration-created schema)
- Task 3 depends on tasks 1+2 (generation needs both migrations and queries)
- Task 5 depends on tasks 3+4 (store needs generated code and config)
- Task 6 depends on tasks 1+4 (CLI uses golang-migrate with embedded files + config, not the store layer)
- Task 7 depends on task 1 (test harness needs migration files)
- Task 8 depends on tasks 5+7 (tests need store and harness)
- Task 9 depends on task 3 (regression check needs generation to be clean)
- Task 10 can start after task 8 (docs after implementation is validated)
