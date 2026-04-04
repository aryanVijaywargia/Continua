# Capability: engine-schema-runtime-delta

Schema migration, new/modified queries, and store methods required for engine runtime execution. Extends [engine-schema-foundation](../../../../changes/add-engine-foundation/specs/engine-schema-foundation/spec.md).

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-runtime-config](../engine-runtime-config/spec.md)

## ADDED Requirements

### Requirement: Runtime migration 000002

The engine MUST provide migration `000002_runtime_columns` that extends the existing schema for runtime execution without creating new tables.

#### Scenario: Enum extension
- **WHEN** the up migration runs
- **THEN** `engine.run_lifecycle_status` gains the value `waiting`

#### Scenario: New columns on engine.runs
- **WHEN** the up migration runs
- **THEN** `engine.runs` gains columns: `result JSONB`, `custom_status JSONB`, `waiting_for JSONB`, `completed_at TIMESTAMPTZ`

#### Scenario: No new tables
- **WHEN** the up migration runs
- **THEN** no new tables are created; only ALTER statements are executed

#### Scenario: No lease_token columns
- **WHEN** the up migration runs
- **THEN** no `lease_token` columns are added to any table

#### Scenario: Down migration safety guard
- **WHEN** the down migration runs and any run has `status = 'waiting'`
- **THEN** the down migration fails with an explicit error rather than silently dropping data

#### Scenario: Down migration enum replacement
- **WHEN** the down migration runs and no run has `status = 'waiting'`
- **THEN** the old enum is renamed, a replacement enum without `waiting` is created, `engine.runs.status` is altered with an explicit cast to the new type, and the old enum is dropped

#### Scenario: Down migration column removal
- **WHEN** the down migration completes
- **THEN** the `result`, `custom_status`, `waiting_for`, and `completed_at` columns no longer exist on `engine.runs`

---

### Requirement: Guarded workflow-owned run transitions

The engine MUST provide CAS queries for workflow-owned transitions that verify both expected status and claimant identity.

#### Scenario: Running to waiting transition
- **WHEN** a guarded transition query sets `status = 'waiting'` with `waiting_for` and `WHERE id = $1 AND status = 'running' AND claimed_by = $2`
- **THEN** the run transitions to `waiting` and returns the updated row if the CAS matches
- **THEN** returns zero rows affected if either the status or claimed_by does not match

#### Scenario: Running to completed transition
- **WHEN** a guarded transition query sets `status = 'completed'` with `result` and `completed_at = NOW()` and `WHERE id = $1 AND status = 'running' AND claimed_by = $2`
- **THEN** the run transitions to `completed` and returns the updated row if the CAS matches

#### Scenario: Running to failed transition
- **WHEN** a guarded transition query sets `status = 'failed'` with error details and `completed_at = NOW()` and `WHERE id = $1 AND status = 'running' AND claimed_by = $2`
- **THEN** the run transitions to `failed` and returns the updated row if the CAS matches

---

### Requirement: Guarded external wakeup transition

The engine MUST provide a CAS query for external wakeups that verifies expected status without requiring claimant identity.

#### Scenario: Waiting to queued transition
- **WHEN** a guarded wakeup query sets `status = 'queued'` with `WHERE id = $1 AND status = 'waiting'`
- **THEN** the run transitions to `queued` and returns the updated row if the status matches
- **THEN** returns zero rows affected if the run is not in `waiting` status (idempotent no-op for already-queued, running, completed, failed, or cancelled runs)

#### Scenario: WakeWaitingRun wrapper no-op
- **WHEN** the guarded wakeup query returns zero rows affected but the run row still exists
- **THEN** the store wrapper reports `applied = false` with no `ErrStaleClaim`

#### Scenario: WakeWaitingRun wrapper missing row
- **WHEN** the guarded wakeup query returns zero rows affected and the run row does not exist
- **THEN** the store wrapper returns `ErrNotFound`

---

### Requirement: CAS activity completion and failure

The activity completion and failure queries MUST CAS on `claimed_by` to prevent stale workers from overwriting results.

#### Scenario: Activity completion with CAS
- **WHEN** `CompleteActivityTask` is called with `WHERE id = $1 AND status = 'claimed' AND claimed_by = $2`
- **THEN** the task completes only if the CAS matches

#### Scenario: Activity failure with CAS
- **WHEN** `FailActivityTask` is called with `WHERE id = $1 AND status = 'claimed' AND claimed_by = $2`
- **THEN** the task fails only if the CAS matches

#### Scenario: Stale claim rejection
- **WHEN** a CAS query returns zero rows affected and the row exists
- **THEN** the store wrapper returns `ErrStaleClaim` (not `ErrNotFound`)

---

### Requirement: Run-scoped inbox queries

The engine MUST provide queries for listing and consuming inbox items scoped to a specific run.

#### Scenario: List pending inbox by run
- **WHEN** `ListPendingInboxByRun` is called with a `run_id` and `available_at <= NOW()`
- **THEN** returns all `pending` inbox rows for that run ordered by `available_at ASC, id ASC`

#### Scenario: Mark inbox processed with status guard
- **WHEN** `MarkInboxProcessed` is called within the activation transaction
- **THEN** the query uses `WHERE id = $1 AND status = 'pending'` to guard against double-processing
- **THEN** the inbox row transitions from `pending` to `processed` with `resolved_at = NOW()`
- **THEN** if the row is not in `pending` status, zero rows are affected (idempotent no-op)

---

### Requirement: ListActivityTasksByRun query

The engine MUST provide a query to list activity tasks for a specific run.

#### Scenario: List activity tasks by run
- **WHEN** `ListActivityTasksByRun` is called with a `run_id`
- **THEN** returns all activity tasks for that run ordered by `created_at ASC, id ASC`

---

### Requirement: Atomic start request dedupe claim

The engine MUST provide a transaction-safe primitive for `start` request deduplication that can claim a new request, surface a finalized duplicate, detect a live in-progress duplicate, or reclaim an expired claim without racing the unique constraint on `(project_id, request_scope, request_key)`.

#### Scenario: Claim new start request
- **WHEN** the start dedupe primitive runs and no row exists for `(project_id, request_scope = 'engine.start', request_key)`
- **THEN** it creates and returns a row with `status = 'in_progress'` and a fresh `expires_at`

#### Scenario: Return finalized duplicate
- **WHEN** the start dedupe primitive finds an existing row with `status = 'completed'` or `status = 'failed'`
- **THEN** it returns that existing finalized row without creating or replacing a second row

#### Scenario: Live in-progress duplicate
- **WHEN** the start dedupe primitive finds an existing row with `status = 'in_progress'` and `expires_at >= NOW()`
- **THEN** it returns that existing live claim so the caller can surface `request_in_progress`

#### Scenario: Expired in-progress takeover
- **WHEN** the start dedupe primitive finds an existing row with `status = 'in_progress'` and `expires_at < NOW()`
- **THEN** it atomically renews that same logical claim with a refreshed `expires_at` and cleared finalized-response fields
- **THEN** it does not rely on an unlocked get-then-insert or delete-then-insert pattern

#### Scenario: Reclaim previously expired row
- **WHEN** the start dedupe primitive finds an existing row with `status = 'expired'`
- **THEN** it atomically renews that same logical claim with `status = 'in_progress'`, a refreshed `expires_at`, and cleared finalized-response fields
- **THEN** the same `request_key` becomes usable again after maintenance expiry

---

### Requirement: Request dedupe expiry query

The engine MUST provide a query to expire stale request dedupe rows.

#### Scenario: Expire stale dedupe rows
- **WHEN** `ExpireRequestDedupe` is called
- **THEN** rows with `status = 'in_progress'` and `expires_at < NOW()` are transitioned to `expired`
- **THEN** returns the count of expired rows

---

### Requirement: ErrStaleClaim sentinel error

The store MUST provide `ErrStaleClaim` to distinguish stale ownership from a genuinely missing row.

#### Scenario: CAS miss on existing row
- **WHEN** a CAS update returns zero rows affected and the row exists in the table
- **THEN** the store wrapper returns `ErrStaleClaim`

#### Scenario: CAS miss on missing row
- **WHEN** a CAS update returns zero rows affected and the row does not exist
- **THEN** the store wrapper returns `ErrNotFound`

#### Scenario: Status-only wakeup is not a stale-claim path
- **WHEN** `WakeWaitingRun` finds an existing row that is no longer in `waiting`
- **THEN** the wrapper reports an idempotent no-op rather than `ErrStaleClaim`
