# Capability: engine-runtime-execution

Purge / repair service additions to the platform-side engine control layer: defines the purge service contract (terminal-only, mode-gated, CAS-guarded, shared by public API and retention) and the repair-request contract (single-run, async, scoped to catching_up recovery, structured result).

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-public-api](../engine-public-api/spec.md), [engine-schema-runtime-delta](../engine-schema-runtime-delta/spec.md)

## ADDED Requirements

### Requirement: Purge service semantics

The purge service MUST be terminal-only, mode-gated, barrier-serialized, and must never delete the operator shell.

#### Scenario: Terminal-only gate
- **WHEN** the purge service receives a request for a run
- **THEN** it checks the run's `engine_run_status` via the engine runs table
- **WHEN** the status is not one of `completed`, `failed`, `cancelled`, `terminated`
- **THEN** the service returns the existing typed `run_not_terminal` error and performs no deletion
- **THEN** the API handler maps this error to HTTP 409

#### Scenario: Shared purge path for API and retention
- **WHEN** purge is invoked from the public API or the retention worker
- **THEN** the same purge service implementation performs the terminal gate, mode dispatch, CAS barrier handling, and row deletion
- **THEN** no separate retention-only purge logic is introduced

#### Scenario: Mode dispatch
- **WHEN** the purge service is called with `mode=projection_only`
- **THEN** it performs the projection-only delete + barrier flip to `summary_only`
- **WHEN** the purge service is called with `mode=full`
- **THEN** it performs the projection-only delete, then the engine history delete, then the barrier flip to `journal_expired`

#### Scenario: Barrier CAS under row lock
- **WHEN** the purge service executes
- **THEN** it opens a transaction that takes `SELECT ... FOR UPDATE` on the target `public.traces` row
- **THEN** it reads `engine_projection_state` inside the transaction and refuses to downgrade a stronger barrier
- **THEN** `projection_only` on a trace already at `journal_expired` is a no-op
- **THEN** `projection_only` on a trace already at `summary_only` is a no-op (no re-delete, no state change)
- **THEN** `full` on a trace already at `journal_expired` is a no-op

#### Scenario: Projection delete scope
- **WHEN** the purge service deletes detail
- **THEN** only rows tied to the target run's trace are deleted
- **THEN** the trace row and the trace's root span are never deleted
- **THEN** no spans belonging to other traces are affected

#### Scenario: Engine history delete scope for full purge
- **WHEN** the purge service deletes engine history (only in `full` mode)
- **THEN** only `engine.history` rows with the target `run_id` are deleted
- **THEN** the `engine.runs` row is preserved
- **THEN** the `engine.instances` row is preserved

#### Scenario: Purge is single-run
- **WHEN** the purge service executes
- **THEN** exactly one run's detail / history is touched per call
- **THEN** no bulk purge path exists in this capability

#### Scenario: Purge emits structured result
- **WHEN** purge succeeds
- **THEN** the service returns `{run_id, mode, projection_state, deleted: boolean}` (or equivalent structured result)
- **THEN** `deleted=true` when rows were deleted, `deleted=false` for idempotent no-op calls

---

### Requirement: Shared control orchestration is provided outside the API server

The platform MUST expose purge/repair orchestration as an Fx-provided shared service outside the private API server constructor so both API handlers and background jobs can depend on the same implementation.

#### Scenario: API handlers use shared service
- **WHEN** the API server is constructed
- **THEN** it receives the shared control service through dependency injection
- **THEN** it does NOT privately construct purge/repair orchestration inside `newConfiguredServer`

#### Scenario: Retention worker uses the same shared service
- **WHEN** the retention worker is constructed
- **THEN** it receives the same shared control service through dependency injection
- **THEN** it does NOT duplicate purge/repair orchestration or self-call the HTTP API

---

### Requirement: Repair request semantics

The repair service MUST be single-run, MUST be async against the current runtime split, MUST reuse the existing projector catch-up path indirectly by resuming `catching_up`, MUST be scoped to catching_up recovery (no full checkpoint rewind), and MUST return a structured result.

#### Scenario: Single-run entry
- **WHEN** the repair service receives a `run_id`
- **THEN** it resolves the trace for that run via the same project-scoped lookup used by other engine handlers
- **THEN** it operates on exactly that one trace

#### Scenario: Repair dispatches on current projection state
- **WHEN** the trace is `up_to_date`
- **THEN** the service returns `{accepted: false, reason: "already_up_to_date"}` without invoking the projector
- **WHEN** the trace is `summary_only`
- **THEN** the service checks whether retained history exists beyond the checkpoint
- **THEN** if retained history exists, it clears the barrier (state transitions to `catching_up`) and returns `{accepted: true, reason: "repair_requested", projection_state: "catching_up"}`
- **WHEN** the trace is `journal_expired`
- **THEN** the service returns `{accepted: false, reason: "history_expired"}` without clearing the barrier
- **WHEN** the trace is `catching_up`
- **THEN** the service returns `{accepted: true, reason: "already_catching_up", projection_state: "catching_up"}`

#### Scenario: Checkpoint-forward resume
- **WHEN** repair is accepted for a `summary_only` trace
- **THEN** the existing `continua-engine` projector later applies events from `engine_last_projected_history_id + 1` through `engine_latest_history_id`
- **THEN** repair does NOT rewind the checkpoint to 0 by default
- **THEN** if the checkpoint equals `engine_latest_history_id` (no events to rebuild), the service returns `{accepted: false, reason: "no_events_to_project"}` and the barrier is preserved

#### Scenario: Repair returns current, not final, projection state
- **WHEN** repair returns
- **THEN** the response includes the current `projection_state`
- **THEN** accepted repair requests report `projection_state = 'catching_up'`
- **THEN** `up_to_date` is reported only by later read APIs after the separate projector loop finishes catch-up

#### Scenario: Repair requests converge
- **WHEN** repair runs multiple times against the same run
- **THEN** the service does not create duplicate detail rows
- **THEN** once a run is already `catching_up` or `up_to_date`, later responses reflect that current state instead of restarting rebuild

#### Scenario: Repair never bypasses projector writes
- **WHEN** repair executes
- **THEN** the repair service itself does not write projection detail rows
- **THEN** all later detail writes still flow through the separate projector's existing write path
- **THEN** no handwritten SQL writes detail rows directly from the repair service

---

### Requirement: Purge and repair handlers honor project scoping

API handlers for purge and repair MUST derive `project_id` from the authenticated API key and MUST reject cross-project runs with 404.

#### Scenario: Project scoping from API key
- **WHEN** a purge or repair handler runs
- **THEN** `project_id` is taken from the API-key auth context
- **THEN** the run lookup filters by `project_id`
- **THEN** a run belonging to a different project is indistinguishable from a missing run (returns 404)

#### Scenario: No bypass via run id
- **WHEN** a caller supplies a valid `run_id` that exists in another project
- **THEN** the handler returns 404
- **THEN** no purge or repair action is taken
