# Capability: engine-trace-projection

Operational hardening additions to the engine trace projection: extends the `public.traces.engine_run_status` CHECK constraint to accept `terminated`, defines the projection mapping for terminated runs, and specifies that the projector itself performs terminal debugger cleanup (closing open activity spans, emitting synthetic wait-resolution events, clearing `engine_wait_state`) when it processes `workflow.cancelled`/`workflow.terminated` history rows, so the debugger never shows a terminal run as still waiting.

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-history-events](../engine-history-events/spec.md)

## ADDED Requirements

### Requirement: Platform CHECK-constraint migration accepts terminated

The platform database MUST ship a migration that drops and recreates `traces_engine_run_status_check` so `public.traces.engine_run_status` accepts the value `'terminated'`.

#### Scenario: Up migration replaces the CHECK constraint
- **WHEN** the up migration runs
- **THEN** `traces_engine_run_status_check` is dropped and recreated
- **THEN** the recreated constraint accepts the values `('queued','running','waiting','completed','failed','cancelled','terminated')`
- **THEN** rows already in the table are not altered

#### Scenario: Down migration requires data fix
- **WHEN** the down migration runs while any row has `engine_run_status = 'terminated'`
- **THEN** the down migration fails with an explicit error naming the offending rows
- **THEN** rollback must first re-transition those rows away from `terminated`

#### Scenario: Down migration restores original constraint
- **WHEN** the down migration runs and no row has `engine_run_status = 'terminated'`
- **THEN** the CHECK constraint is recreated with the original value set (no `terminated`)

---

### Requirement: Terminated projection mapping

The projector MUST map the engine's `terminated` run status to a clear projected triple.

#### Scenario: Raw trace status mapping
- **WHEN** the projector projects a terminal `terminated` run
- **THEN** the raw `public.traces.status` is set to `failed`
- **THEN** this uses an existing allowed value so no additional trace status enum migration is required

#### Scenario: Root span status mapping
- **WHEN** the projector closes the root span for a `terminated` run
- **THEN** the root span `status` is set to `failed`

#### Scenario: Engine summary status mapping
- **WHEN** the projector writes the projected engine summary for a `terminated` run
- **THEN** `public.traces.engine_run_status` is set to `terminated`
- **THEN** read endpoints returning `EngineRunStatus` return `TERMINATED` for that trace

#### Scenario: Failure details on terminated
- **WHEN** the projector writes the projected engine summary for a `terminated` run
- **THEN** the summary includes `error_code = 'terminated'` and `error_message = 'run terminated by operator'`

---

### Requirement: Projector performs terminal cleanup on terminal history rows

The projector MUST perform debugger cleanup (close open activity spans, emit synthetic wait-resolution events, clear `engine_wait_state`) as part of processing `workflow.cancelled` and `workflow.terminated` history rows. Terminal cleanup is NOT performed inside the engine-side activation transaction or the operator terminate transaction.

#### Scenario: Projector is the single writer for terminal cleanup
- **WHEN** the projector processes a `workflow.cancelled` or `workflow.terminated` history row
- **THEN** the projector performs all debugger cleanup writes to `public.spans`, `public.span_events`, and `public.traces`
- **THEN** no other code path writes terminal cleanup to projection tables
- **THEN** this preserves the single-writer invariant for projection tables and avoids any race between the terminal transaction and concurrent projection progress

#### Scenario: Cleanup inputs are read from engine state
- **WHEN** the projector performs terminal cleanup
- **THEN** the activity-task rows to close are loaded through the explicit cancelled-activity read query (`run_id = ? AND status = 'cancelled'`)
- **THEN** the inbox rows whose waits should be resolved are loaded through the explicit discarded-timer read query (`run_id = ? AND status = 'discarded' AND kind = 'timer'`)
- **THEN** the current wait state is reconstructed from prior projected state: activity/timer waits come from earlier projected history handling, and pure signal waits come from the existing projected `engine_wait_state` column rather than a dedicated signal-wait history event
- **THEN** no pre-transition `waiting_for` snapshot is carried across the terminal transaction

#### Scenario: Close still-running activity spans
- **WHEN** the projector processes terminal cleanup and finds cancelled activity-task rows
- **THEN** for each projected activity span that is still open, it sets the span `status` to `failed` and sets `end_time` derived from the terminal history row timestamp
- **THEN** `status_message` is set to the terminal reason message (`workflow cancelled` or `run terminated by operator`)
- **THEN** if an activity span is already closed (already `completed` or `failed`), it is not re-closed

#### Scenario: Synthetic wait-resolution for activity waits
- **WHEN** the projector processes cancelled activity-task rows during terminal cleanup
- **THEN** for each cancelled activity, it emits a synthetic `EventType="wait"` event into `public.span_events` anchored to the terminal history row
- **THEN** the payload uses the existing wait shape `{wait_kind:"activity", phase:"resolved", wait_id:"activity:<activity_key>", resolution:<terminal_reason>}`
- **THEN** `<terminal_reason>` is `cancelled` for `workflow.cancelled` and `terminated` for `workflow.terminated`

#### Scenario: Synthetic wait-resolution for timer waits
- **WHEN** the projector processes discarded inbox rows with `kind = 'timer'` during terminal cleanup
- **THEN** for each timer row, it emits a synthetic `EventType="wait"` event into `public.span_events` anchored to the terminal history row
- **THEN** the payload uses the existing wait shape `{wait_kind:"timer", phase:"resolved", wait_id:"timer:<timer_key>", resolution:<terminal_reason>}`
- **THEN** `<terminal_reason>` is `cancelled` for `workflow.cancelled` and `terminated` for `workflow.terminated`

#### Scenario: Signal waits are cleared via engine_wait_state only
- **WHEN** the projector finds from its reconstructed state that the run was on a signal wait at terminal
- **THEN** the projector does NOT emit a synthetic `wait_kind='signal'` `phase='resolved'` event into `public.span_events`
- **THEN** signal-wait visibility is cleared by nulling the existing `engine_wait_state` column on the projected trace (see "Engine summary wait state is cleared on terminal")
- **THEN** the projector does not invent a new signal-wait timeline model as part of terminal hardening

#### Scenario: Cancel inbox rows do not emit wait-resolution
- **WHEN** the projector processes discarded inbox rows with `kind = 'cancel'` during terminal cleanup
- **THEN** no synthetic wait-resolution event is emitted for those rows
- **THEN** cancel inbox rows are not treated as a user-observable wait

#### Scenario: Idempotency via VariantKey
- **WHEN** the projector emits a synthetic event during terminal cleanup
- **THEN** the event's `VariantKey` is derived deterministically from `terminal_reason + ":" + wait_id`
- **THEN** re-projecting the same terminal history row produces no new rows (upsert/do-nothing)

#### Scenario: Anchored to terminal history row
- **WHEN** the projector writes synthetic events or closes spans during terminal cleanup
- **THEN** every write is tagged with the terminal history row ID for traceability
- **THEN** terminal cleanup can be reconstructed from the terminal history row alone

#### Scenario: Synthetic cleanup sequence offsets use the terminal row base
- **WHEN** the projector emits synthetic cleanup events for a terminal history row with sequence number `N`
- **THEN** the terminal history row keeps its normal projected base sequence `N*10`
- **THEN** synthetic cleanup wait events use monotonically increasing sequence offsets starting at `N*10 + 1`
- **THEN** activity cleanup events are assigned first in query order, followed by timer cleanup events in query order
- **THEN** because the terminal history row is the last row for the run, these offsets do not collide with later projected history events

#### Scenario: No race with earlier scheduling events
- **WHEN** the projector processes history in order for a run
- **THEN** all `activity.scheduled` / `timer.scheduled` / `signal.received` events relevant to the run's current projected wait state are processed before the terminal history row
- **THEN** terminal cleanup writes are the last projection writes for the run
- **THEN** no later projection write for the run can overwrite cleanup state, because the run is terminal and no further history rows will be appended

---

### Requirement: Engine summary wait state is cleared on terminal

The existing projected engine summary column `engine_wait_state` (Go type `EngineWaitState`) MUST be cleared when a run reaches any terminal status.

#### Scenario: engine_wait_state is cleared on terminal
- **WHEN** a run transitions to `completed`, `failed`, `cancelled`, or `terminated`
- **THEN** the projector writes NULL to `public.traces.engine_wait_state` for the run's projected trace
- **THEN** no new projection column is introduced; this uses the existing `engine_wait_state` storage and `EngineWaitState` mapper

#### Scenario: engine_pending_activity_tasks count on terminal
- **WHEN** the projector syncs the summary after terminal sealing
- **THEN** `engine_pending_activity_tasks` reflects the actual open activity rows (zero after sealing)
- **THEN** terminated runs do not display lingering open activities

#### Scenario: engine_pending_inbox_items count on terminal
- **WHEN** the projector syncs the summary after terminal sealing
- **THEN** `engine_pending_inbox_items` reflects the actual open inbox rows (zero after sealing, excluding `cancel` rows per existing semantics)

---

### Requirement: Synthetic cleanup events are projection-only

Synthetic terminal cleanup events MUST live in the projection layer and never be mirrored back into `engine.history`.

#### Scenario: No engine.history rows are appended
- **WHEN** the projector emits synthetic wait-resolution events during terminal cleanup
- **THEN** no new rows are appended to `engine.history` for those events
- **THEN** only the terminal history row (`workflow.cancelled` or `workflow.terminated`) remains in engine history

#### Scenario: Synthetic events go only to public.span_events
- **WHEN** the projector emits synthetic events during terminal cleanup
- **THEN** the rows are written to `public.span_events` with a recognizable kind/tag for debugger consumers
- **THEN** no other projection table receives duplicate rows for the same synthetic event

#### Scenario: Replay is unaffected
- **WHEN** a run with synthetic cleanup events is read for replay
- **THEN** replay consumes only `engine.history`
- **THEN** synthetic cleanup events do not alter replay's event sequence or decisions
