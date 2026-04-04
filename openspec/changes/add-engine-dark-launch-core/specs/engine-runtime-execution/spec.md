# Capability: engine-runtime-execution

Worker loops, activation transaction, replay-from-history, guarded state transitions, and activity execution for the engine runtime.

Related capabilities: [engine-schema-runtime-delta](../engine-schema-runtime-delta/spec.md), [engine-history-events](../engine-history-events/spec.md), [engine-workflow-authoring](../engine-workflow-authoring/spec.md)

## ADDED Requirements

### Requirement: Three worker loops

The `serve` command MUST run exactly three polling loops: workflow, activity, and maintenance.

#### Scenario: Workflow worker loop
- **WHEN** `serve` starts
- **THEN** a workflow worker loop polls for claimable runs at the configured `ENGINE_WORKFLOW_POLL_INTERVAL`
- **THEN** each iteration generates a unique `claimed_by` identity like `workflow:<uuid>`

#### Scenario: Activity worker loop
- **WHEN** `serve` starts
- **THEN** an activity worker loop polls for claimable activity tasks at the configured `ENGINE_ACTIVITY_POLL_INTERVAL`
- **THEN** each iteration generates a unique `claimed_by` identity like `activity:<uuid>`

#### Scenario: Maintenance worker loop
- **WHEN** `serve` starts
- **THEN** a maintenance worker loop polls at the configured `ENGINE_MAINTENANCE_POLL_INTERVAL`
- **THEN** each iteration generates a unique `claimed_by` identity like `maintenance:<uuid>`

---

### Requirement: Workflow activation transaction

Each workflow activation MUST execute as a single database transaction.

#### Scenario: Activation load phase
- **WHEN** a workflow worker claims a run
- **THEN** within a single transaction, it loads: the claimed run, its instance, ordered history, run-scoped activity tasks, and due pending inbox rows

#### Scenario: Activation replay phase
- **WHEN** history is loaded
- **THEN** the replay engine re-executes the workflow definition against recorded history events, comparing primitive kind, stable key, and typed payload fields

#### Scenario: Replay bootstraps workflow input
- **WHEN** replay starts for a run
- **THEN** the initial `workflow.started` event seeds definition metadata and `Context.Input(...)` before user workflow code advances past the history frontier

#### Scenario: Activation fold phase
- **WHEN** replay reaches the end of recorded history
- **THEN** durable activity outcomes (completed and failed activity tasks) and pending inbox items (signals, cancels, timer fires) are folded into the workflow context

#### Scenario: Activity outcomes become history events
- **WHEN** activation observes a completed or failed activity task after replay reaches the history frontier
- **THEN** it appends `activity.completed` or `activity.failed` to history before user code continues past that `Activity(...)` call
- **THEN** the appended event becomes the deterministic replay source for later activations

#### Scenario: Primitive scheduling becomes history
- **WHEN** workflow execution first blocks on `Activity(...)` or `SleepUntil(...)`
- **THEN** activation appends `activity.scheduled` or `timer.scheduled` before creating the corresponding wake source and transitioning the run to `waiting`

#### Scenario: Inbox consumption becomes history
- **WHEN** activation consumes due timer, signal, or cancel inbox rows after replay reaches the history frontier
- **THEN** it appends `timer.fired`, `signal.received`, or `cancel.requested` before marking those inbox rows processed
- **THEN** user workflow code observes the effect only through those appended history events

#### Scenario: Custom status becomes history
- **WHEN** workflow code calls `SetCustomStatus(...)` during an activation
- **THEN** activation appends `custom_status.updated` in the same transaction that rewrites `runs.custom_status`

#### Scenario: Terminal outcome becomes history
- **WHEN** workflow execution reaches a terminal success or failure path
- **THEN** activation appends `workflow.completed` or `workflow.failed` in the same transaction that transitions the run to its terminal lifecycle status

#### Scenario: Activation commit phase
- **WHEN** the workflow yields a new primitive or completes
- **THEN** within the same transaction: scheduled, consumed, status, and terminal history rows are appended at their defined write points, materialized caches (`result`, `custom_status`, `waiting_for`) are rewritten, consumed inbox rows are marked processed, and the run transitions to `waiting`, `completed`, or `failed`

#### Scenario: Activation atomicity
- **WHEN** any step within the activation transaction fails
- **THEN** the entire transaction rolls back and no state changes are persisted

---

### Requirement: Replay-from-history

The runtime MUST replay workflow execution from history on every activation.

#### Scenario: Replay happy path
- **WHEN** a workflow is activated and history contains events matching the workflow's execution path
- **THEN** replay succeeds and execution continues from the point after the last recorded event

#### Scenario: Workflow input is history-backed
- **WHEN** workflow code reads input through `Context.Input(...)`
- **THEN** the value comes from the recorded `workflow.started` payload, not from ephemeral CLI process state

#### Scenario: Replay mismatch detection
- **WHEN** replay encounters a primitive call that does not match the next recorded history event (different kind, key, or typed payload fields)
- **THEN** a `workflow.replay_mismatch` event is appended to history and the run is failed terminally

#### Scenario: Version mismatch
- **WHEN** a workflow is activated and the definition version does not exactly match `runs.definition_version`
- **THEN** the activation fails the run with a compatibility error

#### Scenario: Activity replay pending
- **WHEN** a workflow calls `Activity()` during replay and the next matching history event is `activity.scheduled` with no later matching `activity.completed` or `activity.failed`
- **THEN** replay treats the call as still pending and activation continues to rely on the durable activity task state instead of re-scheduling

#### Scenario: Activity replay success
- **WHEN** a workflow calls `Activity()` during replay and matching `activity.scheduled` then `activity.completed` events exist in history
- **THEN** the recorded output is provided from history without re-executing the activity handler

#### Scenario: Activity replay failure
- **WHEN** a workflow calls `Activity()` during replay and matching `activity.scheduled` then `activity.failed` events exist in history
- **THEN** the recorded failure is returned from the primitive without re-executing the activity handler

#### Scenario: Timer replay pending
- **WHEN** a workflow calls `SleepUntil()` during replay and the next matching history event is `timer.scheduled` with no later matching `timer.fired`
- **THEN** replay treats the call as still pending and activation continues to rely on the durable timer inbox row instead of re-scheduling

#### Scenario: Timer replay fired
- **WHEN** a workflow calls `SleepUntil()` during replay and matching `timer.scheduled` then `timer.fired` events exist in history
- **THEN** the call returns immediately without re-scheduling

#### Scenario: Signal replay delivery
- **WHEN** a workflow calls `ReceiveSignal()` during replay and a matching `signal.received` event exists in history
- **THEN** the payload is provided from history without consuming another inbox row

#### Scenario: Cancellation replay delivery
- **WHEN** replay reconstructs workflow state and a `cancel.requested` event exists in history
- **THEN** `CancellationRequested()` returns `true` without requiring a second inbox consume

#### Scenario: Custom status replay state
- **WHEN** replay reconstructs workflow state and one or more `custom_status.updated` events exist in history
- **THEN** the latest recorded value becomes the in-memory custom status before user code continues

---

### Requirement: Run-scoped inbox consumption

All Phase 11 inbox rows MUST be run-scoped and consumed only inside the workflow activation transaction.

#### Scenario: Run-scoped inbox
- **WHEN** an inbox item is created (signal, cancel, or timer)
- **THEN** `run_id` is always set on the inbox row

#### Scenario: Inbox consumption boundary
- **WHEN** inbox items are consumed
- **THEN** consumption happens only inside the workflow activation transaction via `MarkInboxProcessed`

---

### Requirement: Activity worker execution model

The activity worker MUST execute handlers outside a database transaction with at-least-once semantics.

#### Scenario: Activity claim and execute
- **WHEN** the activity worker claims a task
- **THEN** it executes the registered handler outside a transaction

#### Scenario: Activity completion with CAS
- **WHEN** the activity handler succeeds
- **THEN** the worker completes the task with `WHERE id = $1 AND status = 'claimed' AND claimed_by = $2`
- **THEN** on success, the worker wakes the owning run with guarded `waiting -> queued`

#### Scenario: Activity failure with CAS
- **WHEN** the activity handler fails
- **THEN** the worker fails the task with the same CAS pattern
- **THEN** on CAS success, the worker wakes the owning run with guarded `waiting -> queued` so activation can observe the failure and decide how to proceed

#### Scenario: Stale claim handling
- **WHEN** activity completion or failure returns `ErrStaleClaim`
- **THEN** the worker drops the stale result silently (logs the event)

---

### Requirement: Maintenance worker scope

The maintenance worker MUST handle timer wakeups and request dedupe expiry only.

#### Scenario: Timer wakeup
- **WHEN** the maintenance worker runs
- **THEN** it finds `waiting` runs that have due pending timer inbox items (`available_at <= NOW()`)
- **THEN** it wakes those runs with guarded `waiting -> queued`

#### Scenario: Request dedupe expiry
- **WHEN** the maintenance worker runs
- **THEN** it expires stale `request_dedupe` rows where `status = 'in_progress'` and `expires_at < NOW()`

#### Scenario: Maintenance worker boundaries
- **WHEN** the maintenance worker runs
- **THEN** it does NOT append history events, write projections, or perform control-plane behavior

---

### Requirement: Single-primitive blocking

Phase 11 workflows MUST block on exactly one primitive at a time.

#### Scenario: Single wait
- **WHEN** a workflow calls `Activity()`, `SleepUntil()`, or `ReceiveSignal()` and the primitive is not yet resolved
- **THEN** the activation transaction creates the corresponding durable wait registration before transitioning the run to `waiting` with `waiting_for` set to a single tagged JSON object
- **THEN** activity waits create an activity task row, timer waits create a timer inbox row with `kind = 'timer'` and `available_at = due_at`, and signal waits are durably registered by the `waiting_for` value itself until a future `signal` command inserts the wakeup inbox row

#### Scenario: Waiting_for structure
- **WHEN** a run transitions to `waiting`
- **THEN** `waiting_for` is one of:
  - `{"kind":"activity","activity_key":"...","activity_type":"..."}`
  - `{"kind":"timer","timer_key":"...","due_at":"RFC3339"}`
  - `{"kind":"signal","signal_name":"..."}`

---

### Requirement: ClaimNextRun behavior with waiting status

`ClaimNextRun` MUST NOT reclaim `waiting` runs. Only `queued` (ready) and `running` (expired lease) runs are claimable.

#### Scenario: Waiting runs excluded from claim
- **WHEN** `ClaimNextRun` polls for eligible runs
- **THEN** it targets `(status = 'queued' AND ready_at <= NOW()) OR (status = 'running' AND lease_expires_at < NOW())`
- **THEN** runs with `status = 'waiting'` are never claimed by this query

---

### Requirement: UpdateRunStatus exclusion from runtime

`UpdateRunStatus` MUST NOT be used in runtime execution paths. It remains legacy/admin/test-only.

#### Scenario: Runtime uses guarded transitions
- **WHEN** the runtime needs to transition a run's status
- **THEN** it uses the guarded CAS transition queries, never `UpdateRunStatus`
