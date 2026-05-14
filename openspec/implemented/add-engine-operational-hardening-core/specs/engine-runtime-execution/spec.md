# Capability: engine-runtime-execution

Operational hardening of the activation transaction and control-plane handlers: documents the existing `workflow.ErrCancelled` sentinel with explicit replay semantics, imposes ordering guarantees on `decisionCancelled`, introduces a direct terminate path outside activation, defines shared terminal sealing via `UPDATE ... RETURNING`, and makes `engine.instances.status` authoritative going forward.

Related capabilities: [engine-history-events](../engine-history-events/spec.md), [engine-schema-runtime-delta](../engine-schema-runtime-delta/spec.md), [engine-trace-projection](../engine-trace-projection/spec.md)

## ADDED Requirements

### Requirement: workflow.ErrCancelled replay contract

The public engine workflow package's already-exported sentinel error `workflow.ErrCancelled` MUST be the explicit cancelled signal that replay consults.

#### Scenario: Sentinel is exported
- **WHEN** workflow authors import `engine/pkg/workflow`
- **THEN** they see an exported `ErrCancelled` sentinel
- **THEN** they can return it from workflow code to signal an explicit cancelled terminal outcome

#### Scenario: Replay checks ErrCancelled before generic failure
- **WHEN** a workflow definition's `Run(...)` returns a non-nil error
- **THEN** replay uses `errors.Is(runErr, workflow.ErrCancelled)` to branch into the cancelled path before any generic failure handling

#### Scenario: ErrCancelled produces decisionCancelled
- **WHEN** replay identifies `workflow.ErrCancelled`
- **THEN** replay produces `decisionCancelled`, not `decisionFailed`
- **THEN** activation records a terminal `workflow.cancelled` history event (appended or matched during replay)

#### Scenario: Replay mismatch after cancelled
- **WHEN** replay has advanced past `workflow.cancelled` and recorded history contains any further events
- **THEN** replay treats the trailing events as a replay mismatch

#### Scenario: Workflow returns nil after cancel
- **WHEN** the workflow observes cancellation via `CancellationRequested()` and returns `nil`
- **THEN** the run completes normally with `COMPLETED` and `workflow.completed` in history
- **THEN** cancelled is NOT inferred from the cancel request alone

---

### Requirement: decisionCancelled ordering and sealing

When activation produces `decisionCancelled`, the commit phase MUST process consumed inbox rows before sealing remaining open work.

#### Scenario: Consumed inbox is processed first
- **WHEN** `decisionCancelled` commits
- **THEN** every inbox row listed in `decision.ConsumedInboxIDs` is marked `processed` before any sealing query runs

#### Scenario: Terminal history append
- **WHEN** `decisionCancelled` commits
- **THEN** `workflow.cancelled` is appended to history as the terminal event before the run transition

#### Scenario: Run transition guard
- **WHEN** `decisionCancelled` commits
- **THEN** the transition calls `TransitionRunToCancelled` guarded on `status = 'running'` (no `claimed_by` CAS, because the activation holds the row)
- **THEN** the transition clears `claimed_by`, `claimed_at`, `lease_expires_at`, `waiting_for`, `result`
- **THEN** the transition sets `completed_at = NOW()`, `last_error_code='cancelled'`, `last_error_message='workflow cancelled'`

#### Scenario: Sealing remaining open work
- **WHEN** the transition succeeds
- **THEN** the commit calls `CancelOpenActivityTasksByRun` RETURNING the rows actually cancelled
- **THEN** the commit calls `DiscardOpenInboxItemsByRun` RETURNING the rows actually discarded
- **THEN** inbox rows already marked `processed` earlier in the same transaction are not re-discarded

#### Scenario: Terminal projection cleanup is deferred to projector
- **WHEN** the activation transaction commits with `workflow.cancelled` and the sealed row results
- **THEN** the activation transaction does NOT write to `public.spans`, `public.span_events`, or `public.traces`
- **THEN** debugger cleanup is performed by the projector when it later processes the terminal history row (see engine-trace-projection)
- **THEN** the projector is the single writer for projection tables, preserving the single-writer invariant

#### Scenario: Instance status update
- **WHEN** `decisionCancelled` commits
- **THEN** the same transaction updates `engine.instances.status` for the run's instance to `cancelled`

---

### Requirement: Terminate handler semantics

The platform-side terminate handler MUST stop a run forcefully, idempotently, and directly (not inbox-mediated).

#### Scenario: Terminate locks the run row first
- **WHEN** a terminate request enters the handler
- **THEN** the handler opens a transaction and locks the target run row (`SELECT ... FOR UPDATE`) before any writes

#### Scenario: Idempotency on already-terminal runs
- **WHEN** the locked run row already has a terminal status (`completed`, `failed`, `cancelled`, `terminated`)
- **THEN** the handler commits without appending history or sealing work and returns the current terminal state

#### Scenario: Terminate active run
- **WHEN** the locked run row has status `queued`, `running`, or `waiting`
- **THEN** the same transaction appends `workflow.terminated` to history (payload includes `error_code=terminated`, `error_message=run terminated by operator`)
- **THEN** the same transaction calls `TransitionRunToTerminated` guarded on `status IN ('queued','running','waiting')`
- **THEN** the transition clears `claimed_by`, `claimed_at`, `lease_expires_at`, `waiting_for`, `result`, sets `completed_at = NOW()`, sets `last_error_code='terminated'`, `last_error_message='run terminated by operator'`
- **THEN** the same transaction calls `CancelOpenActivityTasksByRun` RETURNING the mutated rows
- **THEN** the same transaction calls `DiscardOpenInboxItemsByRun` RETURNING the mutated rows
- **THEN** the same transaction updates `engine.instances.status` to `terminated`
- **THEN** the terminate transaction does NOT write to `public.spans`, `public.span_events`, or `public.traces`
- **THEN** debugger cleanup is performed by the projector when it later processes the `workflow.terminated` history row (see engine-trace-projection)

#### Scenario: Zero rows from transition is invariant failure
- **WHEN** `TransitionRunToTerminated` returns zero rows under an active-status lock
- **THEN** this is treated as an invariant failure, not a stale-claim condition
- **THEN** the handler rolls back the transaction and returns an internal error

#### Scenario: Terminate does not use the cancel inbox
- **WHEN** terminate commits
- **THEN** no `cancel` inbox row is created for the run
- **THEN** terminate does not depend on activation running

---

### Requirement: Shared terminal sealing contract

Terminal sealing for any direct terminal stop (terminate, decisionCancelled) MUST be driven by the rows actually returned from `UPDATE ... RETURNING`, not only a pre-read snapshot.

#### Scenario: CancelOpenActivityTasksByRun is the source of truth
- **WHEN** open activity tasks are sealed
- **THEN** `CancelOpenActivityTasksByRun RETURNING *` returns the exact set of rows transitioned to `cancelled`
- **THEN** no other pre-read set is used to synthesize activity cleanup

#### Scenario: DiscardOpenInboxItemsByRun is the source of truth
- **WHEN** open inbox rows are sealed
- **THEN** `DiscardOpenInboxItemsByRun RETURNING *` returns the exact set of inbox rows transitioned to `discarded`
- **THEN** rows already `processed` by earlier steps in the same transaction are not included

#### Scenario: Projector reconstructs wait state from prior projected inputs
- **WHEN** the projector performs terminal cleanup
- **THEN** it reconstructs the run's current wait state from the prior history events and projected wait markers it has already processed (activity.scheduled, timer.scheduled, signal.received, and the existing projected `engine_wait_state`)
- **THEN** no pre-transition `waiting_for` snapshot is carried across transaction boundaries — the terminal transaction is not responsible for passing wait state to the projector
- **THEN** pure signal waits are cleared by nulling the existing `engine_wait_state` projection column, not by emitting a synthetic `span_events` row

#### Scenario: Late activity completion does not lose to terminate
- **WHEN** an activity completes before sealing's row lock
- **THEN** the activity row is not in status `queued` or `claimed` and is not returned by `CancelOpenActivityTasksByRun`
- **THEN** no synthetic cancellation is emitted for that activity

#### Scenario: Late activity completion loses to terminate
- **WHEN** terminate's row lock wins and seals before the activity completion commits
- **THEN** the activity completion path returns a stale-claim or no-op result and does not revive the run

---

### Requirement: Instance status authority after terminal transitions

Every terminal run transition MUST write the matching value to `engine.instances.status` in the same transaction.

#### Scenario: Completed run updates instance status
- **WHEN** a run transitions to `completed`
- **THEN** the same transaction sets the owning instance's `status` to `completed`

#### Scenario: Failed run updates instance status
- **WHEN** a run transitions to `failed`
- **THEN** the same transaction sets the owning instance's `status` to `failed`

#### Scenario: Cancelled run updates instance status
- **WHEN** a run transitions to `cancelled`
- **THEN** the same transaction sets the owning instance's `status` to `cancelled`

#### Scenario: Terminated run updates instance status
- **WHEN** a run transitions to `terminated`
- **THEN** the same transaction sets the owning instance's `status` to `terminated`

#### Scenario: Active instance while latest run is non-terminal
- **WHEN** the latest run for an instance is in a non-terminal state (`queued`, `running`, `waiting`)
- **THEN** the owning instance's status is `active`

---

### Requirement: Terminal transactions do not touch projection tables

The engine-side `decisionCancelled` activation transaction and the root-side terminate transaction MUST NOT write to `public.spans`, `public.span_events`, `public.traces`, or other projection tables. Projection cleanup is owned by the projector alone (see engine-trace-projection).

#### Scenario: decisionCancelled commit touches only engine schema
- **WHEN** `decisionCancelled` commits
- **THEN** all writes land in `engine.*` tables (history, runs, instances, activity_tasks, inbox)
- **THEN** no write lands in `public.*` projection tables

#### Scenario: Terminate commit touches only engine schema
- **WHEN** the terminate handler commits
- **THEN** all writes land in `engine.*` tables (history, runs, instances, activity_tasks, inbox)
- **THEN** no write lands in `public.*` projection tables

#### Scenario: Import boundary preserved by design
- **WHEN** root-side code (`internal/api/engine_control.go`) implements terminate
- **THEN** it imports only `engine/db/gen/go` generated sqlc types and public engine packages
- **THEN** it does NOT import any `engine/internal/*` package
- **THEN** no shared projection helper is needed because the projector owns projection writes
