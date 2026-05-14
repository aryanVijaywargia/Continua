# Capability: engine-schema-runtime-delta

Schema / store additions for retention + purge + repair + trace-search: no new trace-shell columns; adds an engine-side history delete query, root-side cross-schema retention candidate selection, platform-side queries for span/span-event deletion and projection-state CAS writes, and root-side env validation for retention startup.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-retention-maintenance](../engine-retention-maintenance/spec.md), [engine-trace-search](../engine-trace-search/spec.md)

## ADDED Requirements

### Requirement: No new trace-shell columns

This change MUST NOT add new columns to `public.traces` or the engine trace-shell tables.

#### Scenario: Reuse of existing engine columns
- **WHEN** retention, purge, repair, filters, or the Python control client need a projection or identity value
- **THEN** they read from the existing columns already populated by Proposal 1's baseline: `engine_run_id`, `engine_instance_key`, `engine_definition_name`, `engine_definition_version`, `engine_projection_state`, `engine_projection_updated_at`, `engine_run_status`, `engine_wait_state`
- **THEN** no migration adds `projection_purged_at`, `engine_wait_kind`, `engine_last_error_code`, or any other trace-shell column in this phase

#### Scenario: Existing engine columns remain sufficient
- **WHEN** retention selects candidates by completion age
- **THEN** the query uses engine run completion time (for example `engine.runs.completed_at`) in combination with the engine run status classification
- **THEN** no derived or denormalized column on `public.traces` is introduced for this purpose

---

### Requirement: Retention candidate index

This change MUST add one index-only migration on `engine.runs` to support efficient retention candidate queries.

#### Scenario: Index on completed_at with terminal status filter
- **WHEN** the retention worker queries for candidates by completion time
- **THEN** the query is supported by a partial index on `engine.runs(completed_at) WHERE status IN ('completed','failed','cancelled','terminated')`
- **THEN** this avoids a full table scan on `engine.runs` for large databases

#### Scenario: Index-only migration
- **WHEN** the migration runs
- **THEN** only an index is created
- **THEN** no table schema changes, new columns, or existing column modifications are introduced
- **THEN** the migration is backwards-compatible and can be applied without downtime

---

### Requirement: Engine history delete query

The engine MUST provide a query that deletes all `engine.history` rows for a given `run_id` in one statement.

#### Scenario: Delete by run id
- **WHEN** the engine history delete query runs with a `run_id`
- **THEN** all `engine.history` rows with that `run_id` are deleted in a single `DELETE` statement
- **THEN** no cursor or paginated delete path is used for single-run history purges

#### Scenario: Query leaves runs and instances intact
- **WHEN** the history delete query runs
- **THEN** no rows are deleted from `engine.runs` or `engine.instances`
- **THEN** the terminal shell remains readable via engine read endpoints

#### Scenario: sqlc annotation
- **WHEN** the query is declared in engine query files
- **THEN** it is annotated as `:exec`
- **THEN** the generated Go function takes `ctx, run_id` and returns an error

---

### Requirement: Root-side retention candidate selection

The root/platform store MUST provide read helpers that list terminal runs eligible for each retention stage, ordered and bounded.

#### Scenario: Stage 1 candidates
- **WHEN** the stage 1 retention helper runs with a threshold timestamp and a limit
- **THEN** it returns terminal runs (`status IN ('completed','failed','cancelled','terminated')`) whose `completed_at < threshold` AND whose projected trace has `engine_projection_state IN ('up_to_date','catching_up')`
- **THEN** rows are ordered by `runs.completed_at ASC, runs.id ASC`
- **THEN** the result set is capped by the provided limit

#### Scenario: Stage 2 candidates
- **WHEN** the stage 2 retention helper runs with a threshold timestamp and a limit
- **THEN** it returns terminal runs whose `completed_at < threshold` AND whose projected trace has `engine_projection_state IN ('summary_only','up_to_date','catching_up')`
- **THEN** rows are ordered by `runs.completed_at ASC, runs.id ASC`
- **THEN** the result set is capped by the provided limit
- **THEN** traces already at `journal_expired` are excluded

#### Scenario: Cross-schema join lives root-side
- **WHEN** candidate selection needs both run status and projection state
- **THEN** the helper joins `engine.runs` with `public.traces` via `engine_run_id`
- **THEN** cross-project scoping is NOT applied at this layer because retention runs across the whole engine
- **THEN** the helper lives in root-side handwritten store code (`internal/store/` or equivalent), not in `engine/db/queries/`, because the current sqlc inputs are schema-local

#### Scenario: Helper preserves ordering and bounds
- **WHEN** the retention worker pages through candidates
- **THEN** the helper preserves the underlying ordering and limit contract
- **THEN** the worker does not reimplement ordering logic outside the helper

---

### Requirement: Projection state CAS writer

The platform store MUST expose a CAS writer that updates `public.traces.engine_projection_state` only when the current state matches an expected set and advances `engine_projection_updated_at`.

#### Scenario: Flip to summary_only under CAS
- **WHEN** purge or retention flips a trace to `summary_only`
- **THEN** the writer updates `engine_projection_state = 'summary_only'` WHERE `engine_run_id = $1 AND engine_projection_state IN ('up_to_date','catching_up')`
- **THEN** `engine_projection_updated_at = NOW()` on the mutated row
- **THEN** zero rows updated means the expected state was not matched (no-op or already stronger barrier)

#### Scenario: Flip to journal_expired under CAS
- **WHEN** purge or retention flips a trace to `journal_expired`
- **THEN** the writer updates `engine_projection_state = 'journal_expired'` WHERE `engine_run_id = $1 AND engine_projection_state IN ('up_to_date','catching_up','summary_only')`
- **THEN** `engine_projection_updated_at = NOW()` on the mutated row

#### Scenario: Flip from summary_only to catching_up for repair
- **WHEN** repair clears the barrier for a `summary_only` trace
- **THEN** the writer updates `engine_projection_state = 'catching_up'` WHERE `engine_run_id = $1 AND engine_projection_state = 'summary_only'`
- **THEN** zero rows updated means the trace was already beyond `summary_only` and repair reports the current state as-is

#### Scenario: Writer never downgrades a stronger barrier
- **WHEN** the writer attempts to flip `journal_expired` back to `summary_only`, `catching_up`, or `up_to_date`
- **THEN** the CAS predicate does NOT match
- **THEN** zero rows are updated and the state remains `journal_expired`

---

### Requirement: Platform detail deletion queries

The platform store MUST provide queries to delete `public.span_events` and non-root `public.spans` for a trace as part of the projection purge path.

#### Scenario: Delete all span events by trace
- **WHEN** the span-event delete query runs with a trace identifier
- **THEN** all rows in `public.span_events` tied to that trace are deleted in a single `DELETE`
- **THEN** no span or trace rows are touched

#### Scenario: Delete non-root spans by trace
- **WHEN** the non-root span delete query runs with a trace identifier
- **THEN** all rows in `public.spans` tied to that trace where the span is NOT the trace's root span are deleted
- **THEN** the root span row remains
- **THEN** the query uses a single `DELETE` with a subquery or NOT EXISTS guard to identify the root span

#### Scenario: Trace row is never touched
- **WHEN** the detail deletion queries run
- **THEN** the `public.traces` row is not inserted, updated, or deleted by these queries
- **THEN** the trace shell is modified only by the separate projection-state CAS writer

#### Scenario: sqlc annotations
- **WHEN** the queries are declared
- **THEN** each is annotated `:exec`
- **THEN** generated Go functions take `ctx, trace_id` and return an error

---

### Requirement: Retention env config validation at startup

The platform server MUST read `ENGINE_PROJECTION_RETENTION_AFTER` and `ENGINE_HISTORY_RETENTION_AFTER` from env (via `internal/config/config.go`) and MUST fail startup when the pairing is invalid.

#### Scenario: Both empty means retention disabled
- **WHEN** both env vars are empty or missing
- **THEN** the retention worker is not scheduled
- **THEN** startup proceeds normally

#### Scenario: Only projection set
- **WHEN** `ENGINE_PROJECTION_RETENTION_AFTER` is set and `ENGINE_HISTORY_RETENTION_AFTER` is empty or zero
- **THEN** stage 1 (projection_only purge) runs and stage 2 is disabled
- **THEN** startup proceeds normally

#### Scenario: History without projection
- **WHEN** `ENGINE_HISTORY_RETENTION_AFTER` is set but `ENGINE_PROJECTION_RETENTION_AFTER` is empty or zero
- **THEN** startup MUST fail fast with an explicit configuration error naming both env vars
- **THEN** the retention worker is NOT scheduled

#### Scenario: History less than or equal to projection
- **WHEN** both env vars are set and `ENGINE_HISTORY_RETENTION_AFTER <= ENGINE_PROJECTION_RETENTION_AFTER`
- **THEN** startup MUST fail fast with an explicit configuration error
- **THEN** the retention worker is NOT scheduled

#### Scenario: Unparseable duration
- **WHEN** either env var is present but cannot be parsed as a Go `time.Duration` string
- **THEN** startup MUST fail fast with an explicit configuration error identifying the offending variable
- **THEN** the retention worker is NOT scheduled

#### Scenario: Zero disables the stage
- **WHEN** either env var is explicitly set to `"0"` or `"0s"`
- **THEN** the corresponding stage is treated as disabled
- **THEN** the `history > projection` rule is evaluated only against stages that are enabled

---

### Requirement: Store helpers respect runtime and schema boundaries

The engine store MUST expose the history delete wrapper only. The root/platform store MUST expose retention candidate selection, projection-state CAS writers, and detail-deletion queries.

#### Scenario: Root-side candidate selection
- **WHEN** the retention worker needs candidates
- **THEN** it calls root-side store helpers that execute the handwritten cross-schema candidate selection
- **THEN** no candidate-selection SQL is added to `engine/db/queries/`

#### Scenario: Engine-side history delete
- **WHEN** the root-side purge service needs to delete engine history
- **THEN** it calls the engine store's `DeleteHistoryByRun` wrapper through the existing engine query layer
- **THEN** the engine-side history delete remains an engine-only concern

#### Scenario: Platform-side CAS writers and detail deletion
- **WHEN** the root-side purge service needs to flip projection state or delete spans/span_events
- **THEN** it calls platform store wrappers in `internal/store/`
- **THEN** the retention worker does not bypass the purge service to write those tables directly
