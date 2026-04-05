# Capability: engine-schema-runtime-delta

Operational hardening additions to the engine schema: adds `terminated` to the run and instance lifecycle enums, introduces non-CAS guarded terminal transitions (`TransitionRunToTerminated` for operator terminate, a narrowed `TransitionRunToCancelled` for `decisionCancelled`), adds row-returning sealing queries, aligns `CountOpenInboxByRun` with the operator-visible definition (excludes `kind='cancel'`), and backfills `engine.instances.status` to match each instance's latest run.

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-trace-projection](../engine-trace-projection/spec.md)

## ADDED Requirements

### Requirement: terminated enum values

The engine MUST extend `engine.run_lifecycle_status` and `engine.instance_lifecycle_status` to include `terminated`.

#### Scenario: run_lifecycle_status gains terminated
- **WHEN** the up migration runs
- **THEN** `engine.run_lifecycle_status` includes the value `terminated`
- **THEN** existing enum values (`queued`, `running`, `waiting`, `completed`, `failed`, `cancelled`) remain unchanged

#### Scenario: instance_lifecycle_status gains terminated
- **WHEN** the up migration runs
- **THEN** `engine.instance_lifecycle_status` includes the value `terminated`
- **THEN** existing enum values (`active`, `completed`, `failed`, `cancelled`) remain unchanged

#### Scenario: Down migration safety guard
- **WHEN** the down migration runs and any row has `status = 'terminated'` in either enum
- **THEN** the down migration fails with an explicit error rather than silently dropping data
- **THEN** the error message names the table(s) that still contain `terminated` rows

#### Scenario: Down migration enum replacement pattern
- **WHEN** the down migration runs and no row uses `terminated`
- **THEN** the old enum is renamed, a replacement enum without `terminated` is created, the column is altered with an explicit cast to the new type, and the old enum is dropped
- **THEN** the pattern mirrors the existing `waiting` enum down migration

---

### Requirement: TransitionRunToTerminated query

The engine MUST provide a status-guarded (non-CAS on `claimed_by`) transition query that moves a run from an active status to `terminated` and clears claim and wait fields in one update.

#### Scenario: Active run transitions to terminated
- **WHEN** `TransitionRunToTerminated` runs with a run whose `status IN ('queued','running','waiting')`
- **THEN** the row is updated to `status = 'terminated'` and the updated row is returned
- **THEN** `claimed_by`, `claimed_at`, `lease_expires_at`, `waiting_for`, `result` are set to NULL
- **THEN** `completed_at = NOW()`
- **THEN** `last_error_code = 'terminated'` and `last_error_message = 'run terminated by operator'`

#### Scenario: Non-CAS on claimed_by
- **WHEN** the query's WHERE clause is evaluated
- **THEN** it matches on `id = $1 AND status IN ('queued','running','waiting')`
- **THEN** it does NOT include `claimed_by` in the predicate

#### Scenario: Terminal run is not re-terminated
- **WHEN** `TransitionRunToTerminated` runs against a run whose status is already terminal
- **THEN** zero rows are affected and zero rows are returned
- **THEN** the caller treats zero rows under an active-status lock as an invariant failure (see engine-runtime-execution)

#### Scenario: Query returns full updated row
- **WHEN** the query matches
- **THEN** the query returns all columns needed by the caller (including the pre-update `waiting_for` if captured upstream via SELECT ... FOR UPDATE)

---

### Requirement: TransitionRunToCancelled query (running-only, non-CAS)

The engine MUST update `TransitionRunToCancelled` to drop the `claimed_by` CAS while keeping its scope narrowly guarded on `status = 'running'`. `decisionCancelled` is the only caller and it only fires inside activation on a claimed, running run.

#### Scenario: Status-guarded transition on running only
- **WHEN** `TransitionRunToCancelled` runs with a run whose `status = 'running'`
- **THEN** the row is updated to `status = 'cancelled'` and the updated row is returned
- **THEN** `claimed_by`, `claimed_at`, `lease_expires_at`, `waiting_for`, `result` are set to NULL
- **THEN** `completed_at = NOW()`
- **THEN** `last_error_code = 'cancelled'` and `last_error_message = 'workflow cancelled'`

#### Scenario: No CAS on claimed_by
- **WHEN** the query's WHERE clause is evaluated
- **THEN** it matches on `id = $1 AND status = 'running'`
- **THEN** it does NOT include `claimed_by` in the predicate
- **THEN** the activation transaction (which already validated ownership at load time and holds the row in-transaction) can commit the transition without re-asserting claim identity

#### Scenario: No widening for operator terminate
- **WHEN** operator terminate stops a non-running active run (`queued` or `waiting`)
- **THEN** the terminate path uses `TransitionRunToTerminated`, NOT `TransitionRunToCancelled`
- **THEN** `TransitionRunToCancelled` is never broadened to accept `queued` or `waiting`

#### Scenario: Zero rows under active-status lock is invariant failure
- **WHEN** `TransitionRunToCancelled` is called inside an activation that has already verified `status = 'running'` under lock
- **THEN** zero rows is treated as an invariant failure by the caller
- **THEN** the caller rolls back and returns an internal error (see engine-runtime-execution)

---

### Requirement: CancelOpenActivityTasksByRun query

The engine MUST provide a `UPDATE ... RETURNING *` query that transitions every open activity task for a given run to `cancelled` and returns the exact set of rows mutated.

#### Scenario: Sealing open activity tasks
- **WHEN** `CancelOpenActivityTasksByRun` runs with a `run_id`
- **THEN** activity rows with `status IN ('queued','claimed')` for that run transition to `status = 'cancelled'`
- **THEN** `completed_at = NOW()` is set on each mutated row
- **THEN** the query returns every mutated row via `RETURNING *`

#### Scenario: Late completion is not re-cancelled
- **WHEN** an activity row has already transitioned to `completed` or `failed` before the query runs
- **THEN** that row is not matched by the WHERE clause
- **THEN** that row is not included in the returned set

#### Scenario: Rows returned are the source of truth
- **WHEN** the caller uses the returned rows to drive debugger cleanup
- **THEN** the rows exactly represent the activity tasks that this transaction just cancelled
- **THEN** no additional pre-read set is needed to know what was cancelled

#### Scenario: sqlc annotation
- **WHEN** the query is declared in the engine query files
- **THEN** it is annotated `:many` so sqlc generates a slice return
- **THEN** the return type carries the full row (matching the `activity_tasks` table shape)

---

### Requirement: DiscardOpenInboxItemsByRun query

The engine MUST provide a `UPDATE ... RETURNING *` query that transitions every open inbox row for a given run to `discarded` and returns the exact set of rows mutated.

#### Scenario: Sealing open inbox rows
- **WHEN** `DiscardOpenInboxItemsByRun` runs with a `run_id`
- **THEN** inbox rows with `status IN ('pending','claimed')` for that run transition to `status = 'discarded'`
- **THEN** `resolved_at = NOW()` is set on each mutated row
- **THEN** the query returns every mutated row via `RETURNING *`

#### Scenario: Already-processed rows are not re-discarded
- **WHEN** an inbox row has status `processed` before the query runs (e.g., consumed earlier in the same transaction)
- **THEN** that row is not matched by the WHERE clause
- **THEN** that row is not included in the returned set

#### Scenario: Returned rows include kind
- **WHEN** the caller uses the returned rows to drive debugger cleanup
- **THEN** the returned rows carry `kind` (`timer`, `signal`, `cancel`) so the caller can route cleanup by primitive
- **THEN** the rows exactly represent the inbox rows that this transaction just discarded

#### Scenario: sqlc annotation
- **WHEN** the query is declared in the engine query files
- **THEN** it is annotated `:many` so sqlc generates a slice return
- **THEN** the return type carries the full row (matching the `inbox` table shape)

---

### Requirement: Ordered read query for pending-work activity rows

The engine MUST provide a read query for the pending-work endpoint that returns only open activity-task rows for a run in deterministic scheduling order.

#### Scenario: Open activity-task rows for pending-work
- **WHEN** the pending-work activity query runs with a `run_id`
- **THEN** it returns rows from `engine.activity_tasks` with `run_id = $1` and `status IN ('queued','claimed')`
- **THEN** it does NOT apply `available_at <= NOW()`
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: Query returns full row for API mapping
- **WHEN** the caller maps activity items for `GET /pending-work`
- **THEN** the query returns the full task row so `task_id`, `activity_key`, `activity_type`, `status`, `available_at`, and `attempt_count` can be derived without handwritten SQL

---

### Requirement: Ordered read query for pending-work inbox rows by kind

The engine MUST provide a read query for the pending-work endpoint that returns only open inbox rows for a run and a specific kind in deterministic scheduling order.

#### Scenario: Open timer or signal inbox rows for pending-work
- **WHEN** the pending-work inbox query runs with `run_id` and `kind`
- **THEN** it returns rows from `engine.inbox` with `run_id = $1`, `kind = $2`, and `status IN ('pending','claimed')`
- **THEN** it does NOT apply `available_at <= NOW()`
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: Used only for timer and signal arrays
- **WHEN** the pending-work endpoint calls the query
- **THEN** it uses `kind='timer'` for the `timers` array and `kind='signal'` for the `signals` array
- **THEN** `kind='cancel'` is never used for an operator-visible array

---

### Requirement: Ordered read queries for projector terminal cleanup inputs

The engine MUST provide read queries that let the projector re-read sealed engine rows for terminal cleanup without handwritten SQL.

#### Scenario: Cancelled activity-task rows for projector cleanup
- **WHEN** the projector needs activity cleanup inputs for a terminal run
- **THEN** it uses a query that returns `engine.activity_tasks` rows with `run_id = $1` and `status = 'cancelled'`
- **THEN** rows are ordered by `available_at ASC, id ASC`

#### Scenario: Discarded timer inbox rows for projector cleanup
- **WHEN** the projector needs timer cleanup inputs for a terminal run
- **THEN** it uses a query that returns `engine.inbox` rows with `run_id = $1`, `status = 'discarded'`, and `kind = 'timer'`
- **THEN** rows are ordered by `available_at ASC, id ASC`

---

### Requirement: Instance status backfill migration

The engine MUST ship a one-time data backfill that recomputes `engine.instances.status` from each instance's latest run.

#### Scenario: Backfill computes from latest run
- **WHEN** the data backfill migration runs
- **THEN** for each instance, it reads the latest run (by `created_at DESC, id DESC`)
- **THEN** if the latest run is in a terminal state, `engine.instances.status` is set to match (`completed`, `failed`, `cancelled`, `terminated`)
- **THEN** if the latest run is non-terminal (`queued`, `running`, `waiting`), `engine.instances.status` is set to `active`

#### Scenario: Backfill is a single UPDATE ... FROM
- **WHEN** the backfill runs
- **THEN** it executes as a single `UPDATE engine.instances SET status = ... FROM (SELECT latest run per instance)` statement
- **THEN** it does not iterate per-instance in application code

#### Scenario: Backfill is idempotent
- **WHEN** the backfill is re-applied
- **THEN** rows already at the correct status are not changed in meaning
- **THEN** the backfill can safely run again without error

#### Scenario: Backfill does not touch instances with no runs
- **WHEN** an instance has zero associated run rows
- **THEN** the backfill leaves its status unchanged

---

### Requirement: Runtime instance status updates reuse existing query

Terminal transitions MUST call the existing `UpdateInstanceStatus(id, status)` query (`engine/db/queries/instances.sql`) inside the same transaction as the run transition. No new instance-status writer is introduced.

#### Scenario: Existing query is the writer
- **WHEN** a terminal run transition needs to update the owning instance
- **THEN** the caller invokes the existing `UpdateInstanceStatus` query with `(instance_id, status)`
- **THEN** no new query shape (run-keyed or otherwise) is added for this purpose

#### Scenario: Instance ID is already available
- **WHEN** the runtime reaches a terminal transition
- **THEN** the caller already holds the `instance_id` from the locked run/instance read earlier in the same transaction
- **THEN** the caller passes that `instance_id` to `UpdateInstanceStatus` directly

#### Scenario: Called inside the terminal transaction
- **WHEN** any terminal run transition commits (`completed`, `failed`, `cancelled`, `terminated`)
- **THEN** the `UpdateInstanceStatus` call runs in the same transaction as the run transition
- **THEN** a failure here fails the entire terminal transition

#### Scenario: Accepts terminated value
- **WHEN** the caller passes `status='terminated'` to `UpdateInstanceStatus`
- **THEN** the query succeeds because `engine.instance_lifecycle_status` now includes `terminated` (see "terminated enum values")

---

### Requirement: CountOpenInboxByRun excludes cancel inbox rows

The engine `CountOpenInboxByRun` query MUST exclude rows with `kind = 'cancel'` so the reported count reflects only operator-visible pending work (timer and signal rows), matching the semantic used by the `pending-work` endpoint and by projected `engine_pending_inbox_items` counts.

#### Scenario: Query filter excludes cancel kind
- **WHEN** `CountOpenInboxByRun` runs for a run
- **THEN** the WHERE clause is `run_id = $1 AND status IN ('pending','claimed') AND kind <> 'cancel'`
- **THEN** cancel inbox rows are not counted

#### Scenario: Consistent count across read paths
- **WHEN** the same run is read via the engine run-summary endpoint (which uses `CountOpenInboxByRun`), via the pending-work endpoint, and via the projected `engine_pending_inbox_items` column
- **THEN** all three surfaces return the same number of pending inbox items
- **THEN** cancel inbox rows never contribute to that count in any surface

#### Scenario: Rationale — cancel is engine-internal control
- **WHEN** an operator or history consumer reads pending work
- **THEN** cancel inbox rows are not exposed, because they are an internal control primitive used by `POST /cancel` to wake the run, not a user-observable pending wait
- **THEN** this matches the existing semantic that the projector does not emit `wait_kind='cancel'` events

#### Scenario: Existing callers see the updated semantic
- **WHEN** any caller of `CountOpenInboxByRun` runs against a database with cancel rows
- **THEN** the returned count drops by the number of cancel rows for that run compared to the pre-change behavior
- **THEN** existing callers (e.g., `GetRunResult` summary) inherit the corrected semantic without a new query being introduced
