## Context

Phase 10.1 replaces the placeholder `engine/` module with a working schema, store, CLI, and generation pipeline. It is the prerequisite for all future engine runtime phases but must not touch the live debugger/ingest product path.

### Current State

- `engine/cmd/continua-engine/main.go` prints "not yet implemented"
- `engine/db/migrations/postgres/0000_placeholder.sql` is a comment-only placeholder
- `engine/db/queries/` is empty (`.gitkeep` only)
- `engine/internal/` has five empty placeholder packages: `activity`, `history`, `timer`, `worker`, `workflow`
- `engine/db/sqlc.yaml` is configured but has nothing to generate
- `scripts/generate.sh` conditionally skips engine sqlc when queries are empty
- The engine Go module (`engine/go.mod`) already depends on pgx/v5, golang-migrate, uuid, testify, and Fx

### Constraints

- Zero changes to the live product: `cmd/continua`, `internal/*`, `contracts/*`, `db/platform/*`, `web/*`, `sdks/*`
- Engine module isolation: cannot import root `internal/*` or root `db/gen/go/*`
- Postgres is the only runtime DB target
- Existing migrations (platform) are immutable
- Contract-first: schema DDL is the source of truth; sqlc generates Go types
- `make generate` must succeed with no drift after changes

## Goals / Non-Goals

**Goals**
- Establish the `engine` Postgres schema as a clean namespace for all engine state
- Define the core table set: `instances`, `runs`, `history`, `inbox`, `activity_tasks`, `request_dedupe`, `projection_checkpoints`
- Provide reversible migrations (`up` and `down`) with correct FK dependency ordering
- Build a working store layer with pool management, transaction support, and sqlc-backed queries
- Ship a real `continua-engine` binary with `version` and `migrate` commands
- Make engine sqlc generation unconditional in the build pipeline
- Validate shared-DB coexistence by applying platform + engine migrations in every engine test

**Non-Goals**
- Runtime workflow execution, history replay, or activity dispatch
- Public-facing engine APIs or debugger engine UI
- Cross-schema FKs to `public.projects`
- `ON DELETE CASCADE` behavior
- Worker packages beyond placeholder directories
- Python/TypeScript engine control clients

## Decisions

### Decision 1: Dedicated Schema vs Prefixed Tables

**Choice:** Dedicated `engine` schema (`CREATE SCHEMA engine; CREATE TABLE engine.instances ...`)

**Alternatives considered:**
- `public.engine_instances`, `public.engine_runs`, etc.
- Separate physical database

**Why this choice:**
- Schema-level isolation keeps `\dt public.*` clean and engine tables namespaced without naming prefixes
- Shared physical DB avoids operational complexity of multi-database deploys
- PostgreSQL schemas support cross-schema queries if future phases need joins (e.g., linking engine instances to platform traces)
- `search_path` stays at `public` by default; engine code always uses fully-qualified names for explicitness

### Decision 2: No Cross-Schema FKs

**Choice:** Keep `project_id UUID NOT NULL` on engine tables without an FK to `public.projects`.

**Why:**
- Adding a cross-schema FK creates a deployment coupling: engine migrations must run after platform migrations, and the platform `projects` table shape becomes a hard dependency for the engine module
- Without the FK, engine and platform migrations remain independently orderable — critical for a phase where the deployment model is still solidifying
- `project_id` validity is enforced at whatever application boundary fronts the engine (API middleware, control client, etc.), which does not exist yet in Phase 10.1
- Future phases can add the FK once a stable deployment ordering is established

### Decision 3: Enum Strategy

**Choice:** Define five Postgres enums in the `engine` schema and let sqlc generate corresponding Go types.

**Enums:**
- `engine.instance_lifecycle_status`: `active`, `completed`, `failed`, `cancelled`
- `engine.run_lifecycle_status`: `queued`, `running`, `completed`, `failed`, `cancelled`
- `engine.activity_task_status`: `queued`, `claimed`, `completed`, `failed`, `cancelled`
- `engine.inbox_status`: `pending`, `claimed`, `processed`, `discarded`
- `engine.request_dedupe_status`: `in_progress`, `completed`, `failed`, `expired`

**Why enums over TEXT + CHECK:**
- Postgres enums are a single-source-of-truth that sqlc can map to Go typed constants
- Adding values later is `ALTER TYPE ... ADD VALUE` (non-breaking)
- Removing values is rare and would require a migration anyway regardless of approach

### Decision 4: Claim/Lease Pattern

**Choice:** Standardize `claimed_by TEXT`, `claimed_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ` on all claimable tables (`runs`, `inbox`, `activity_tasks`).

**Claim model:** Claiming a row transitions its status (e.g., `queued` → `running` for runs, `pending` → `claimed` for inbox). An expired lease does NOT reset the status back to `queued`/`pending`. Instead, the claim query targets both fresh rows and expired-lease rows in their claimed status:

```sql
-- Claim next eligible run (fresh or expired-lease reclaim)
UPDATE engine.runs
SET status = 'running', claimed_by = $1, claimed_at = NOW(),
    lease_expires_at = NOW() + $2::interval, attempt_count = attempt_count + 1
WHERE id = (
    SELECT id FROM engine.runs
    WHERE (status = 'queued' AND ready_at <= NOW())
       OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW())
    ORDER BY ready_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;
```

The same dual-condition pattern applies to `inbox` (`pending` + expired `claimed`) and `activity_tasks` (`queued` + expired `claimed`).

**Why status transitions on claim (not lease-column-only):**
- Status reflects observable state: a row with `status = 'running'` IS being worked on (or was, with an expired lease). Consumers querying for pending work see the correct count.
- Reclaim is explicit: the `OR` branch in the claim query documents that expired leases are reclaimable. There is no hidden background process resetting statuses.

**Why `SKIP LOCKED`:** Multiple workers polling the same table need contention-free claim without blocking or deadlocks.

**Why `lease_expires_at` instead of heartbeat:** Leases are simpler to implement and reason about for the foundation phase. Heartbeat-based liveness can be layered on top in runtime phases if needed.

### Decision 5: History Table Design

**Choice:** Append-only with `id BIGINT GENERATED ALWAYS AS IDENTITY` for global allocation ordering and `sequence_no INT` for per-run ordering.

**Why dual ordering:**
- `id` gives monotonic allocation order from the Postgres sequence. Under single-writer conditions (one transaction appending per run at a time), allocation order matches commit order. Under concurrent writers, allocation and commit order can diverge — Transaction A may allocate id=5 before Transaction B allocates id=6, but B can commit first.
- `sequence_no` gives stable per-run order — useful for history replay which always operates within a single run. This ordering IS strict because a single run is processed by one worker at a time.
- `UNIQUE(run_id, sequence_no)` prevents gaps or duplicates within a run.

**Ordering caveat for projections:**
- `projection_checkpoints.last_history_id` uses the global `id` as a cursor. In Phase 10.1, this is sufficient because there is no runtime with concurrent writers.
- Runtime phases that introduce concurrent history appends across multiple runs must define a visibility strategy (e.g., gap detection with bounded retry, or a committed-at fence). This is explicitly deferred — the schema supports it, but the semantics are not frozen here.

### Decision 6: Connection Pool Sizing

**Choice:** Conservative defaults: `MaxConns=10`, `MinConns=2`, `MaxConnLifetime=1h`, `MaxConnIdleTime=30m`, `HealthCheckPeriod=1m`.

**Why conservative:**
- Phase 10.1 only uses the pool for migrations and store tests
- Runtime phases (workers, activity dispatch) will retune upward based on actual concurrency needs
- Starting small avoids wasting connections against shared Postgres

### Decision 7: Test Harness Strategy

**Choice:** Engine-local test helpers that apply platform migrations first, then engine migrations, in the same test database.

**Concrete approach:**
- The engine test helper locates the repo root via `runtime.Caller` (walk up from the helper source file until `go.work` or the root `Makefile` is found) and reads platform migration files from `<repo-root>/db/platform/migrations/postgres/`. This is cwd-independent — Go tests set the working directory to the package under test, so a literal relative path like `../../db/platform/migrations/postgres/` would break from nested engine packages.
- The helper uses `ENGINE_TEST_DATABASE_URL` (with fallback to `TEST_DATABASE_URL`, then a default test URL). It must NOT use `DATABASE_URL` — engine tests run down migrations and destructive schema operations that should never hit the dev database. This follows the same safety pattern as the platform test helper in `internal/testutil/testutil.go`.
- The helper creates a fresh test database (or schema), runs `golang-migrate` with the platform migrations, then runs `golang-migrate` with the engine migrations from `engine/db/migrations/postgres/`.
- This is the same `golang-migrate` library already in `engine/go.mod`, so no new dependencies.
- **PG version assumption:** The platform migration uses `gen_random_uuid()` which is a core PostgreSQL function since PG 13 (no `pgcrypto` extension needed). The test harness assumes PG 13+, which matches the project's existing test infrastructure.

**Why platform-first as the default (not a one-off smoke suite):**
- The default deployment model is shared Postgres, so coexistence must be validated from day one
- If engine migrations accidentally conflict with platform schema objects (e.g., naming collisions, extension conflicts), tests catch it immediately in every run — not just in a suite that might be skipped
- The cost is one extra `golang-migrate` pass per test database, which is bounded by the number of platform migrations (currently 12) and runs in <1s
- Engine tests cannot import root `internal/testutil`, so helpers must be self-contained

### Decision 8: Down Migration Strategy

**Choice:** Drop objects in reverse FK dependency order without `DROP SCHEMA engine CASCADE`.

**Order:** `projection_checkpoints` → `request_dedupe` → `activity_tasks` → `inbox` → `history` → `runs` → `instances` → enums → schema.

**Why no CASCADE:**
- `CASCADE` silently drops objects that may have been added by later migrations, making rollback behavior unpredictable
- Explicit reverse-order drops are self-documenting and fail loudly if dependencies are wrong

### Decision 9: Uniqueness Constraints Before Public APIs

Several uniqueness constraints are frozen in `000001` before public execution APIs exist. These are intentional because they encode core durable execution invariants, not API surface assumptions:

- **`instances(project_id, instance_key)`**: Instance identity is the foundational guarantee of durable execution — "start or get" semantics require exactly one instance per key per project. Temporal, Restate, and Durable Objects all enforce this at the storage level. Relaxing it later would break the core execution model.
- **`activity_tasks(run_id, activity_key)`**: Activity deduplication within a run is what makes activities idempotent across retries. Without this constraint, a crashed-and-resumed run could schedule duplicate activity executions. This is a safety invariant, not an API design choice.
- **`inbox(project_id, dedupe_key) WHERE dedupe_key IS NOT NULL`**: This is explicitly opt-in via nullable `dedupe_key`. Callers that don't need deduplication leave it NULL and the partial unique index doesn't apply. The constraint only activates when the caller explicitly requests idempotent signal delivery.
- **`request_dedupe(project_id, request_scope, request_key)`**: Request-level idempotency is scoped to a project + scope + key triple. The scope dimension (`request_scope`) provides enough flexibility for future API shapes without requiring schema changes.

These constraints are additive (can be dropped later if wrong) but removing them after data exists would require a data migration. The risk of locking in the wrong uniqueness is lower than the risk of missing a safety invariant that leads to duplicate execution.

### Decision 10: Engine `migrate down` Requires Explicit Step Count

The root CLI (`cmd/continua migrate down`) defaults to rolling back 1 step when invoked without arguments. The engine CLI intentionally requires an explicit step count.

**Why differ from the root CLI:**
- The root CLI's default-1 behavior was inherited from its Makefile wrapper (`make migrate-down` hardcodes `down 1`). As a standalone binary, `continua-engine migrate down` with no arguments would invoke golang-migrate's raw `Down()`, which rolls back ALL migrations — destructive and surprising.
- Requiring an explicit count makes the destructive operation self-documenting: `continua-engine migrate down 1` vs `continua-engine migrate down 5` clearly communicates intent.
- This is a new binary where we can set the right UX from scratch. The root CLI's behavior is preserved as-is; engine just sets a stricter default for its own surface.

## Table Summary

| Table | Purpose | Claimable | Key Constraints |
|-------|---------|-----------|-----------------|
| `instances` | Workflow instance lifecycle | No | `UNIQUE(project_id, instance_key)` |
| `runs` | Execution attempts per instance | Yes | `UNIQUE(instance_id, run_number)` |
| `history` | Append-only event log | No | `UNIQUE(run_id, sequence_no)`, identity PK |
| `inbox` | Queued signals/timers | Yes | Partial unique on `(project_id, dedupe_key)` |
| `activity_tasks` | External work items | Yes | `UNIQUE(run_id, activity_key)` |
| `request_dedupe` | Idempotency for external requests | No | `UNIQUE(project_id, request_scope, request_key)` |
| `projection_checkpoints` | Consumer position tracking | No | `UNIQUE(projection_name, scope_key)` |
