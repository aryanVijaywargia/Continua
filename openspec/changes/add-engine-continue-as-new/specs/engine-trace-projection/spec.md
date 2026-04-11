## ADDED Requirements

### Requirement: ContinuedAsNew Projection Mapping
The projector SHALL map `continued_as_new` engine run status to projected raw trace status `completed` and root-span status `completed`.

The `workflow.continued_as_new` event SHALL set `end_time` on the trace and root span.

The terminal trace/root span projection for `continued_as_new` SHALL keep `output = null`; it SHALL NOT synthesize the failure-shaped payload used for failed terminal states.

#### Scenario: Continued run trace status
- **WHEN** a run transitions to `continued_as_new`
- **THEN** the projected trace status is `completed`
- **AND** the projected root-span status is `completed`
- **AND** the projected trace/root `output` remains `null`
- **AND** `end_time` is set on both the trace and root span

### Requirement: Continuation Trace Shell Inherits Presentation Fields
When run N continues as run N+1, the new trace shell SHALL inherit the previous run's session link and presentation fields (`name`, `user_id`, `tags`, `environment`, `release`, `metadata`).

#### Scenario: ContinueAsNew inherits fields
- **WHEN** run N continues as run N+1
- **THEN** run N+1's trace shell inherits `name`, `user_id`, `tags`, `environment`, `release`, and `metadata` from run N's trace
- **AND** run N+1 is linked to the same session as run N

### Requirement: Continuation Trace Shell Created Immediately
The new trace shell and root span SHALL be created within the continuation transaction, not deferred to the projector.

The new trace shell SHALL have `engine_projection_state = up_to_date` and the new `workflow.started` event ID as both `engine_latest_history_id` and `engine_last_projected_history_id`.

The continuation shell SHALL reproduce the same live-shell fields as `StartRun`: trace status `running`, root-span status `running`, `output = null`, trace/root input set to the continuation input, `engine_run_status = queued`, nil `engine_custom_status`, nil `engine_wait_state`, zero pending counts, `engine_instance_key`, and `engine_definition_name` / `engine_definition_version`.

#### Scenario: Immediate trace shell creation
- **WHEN** a continuation transaction commits
- **THEN** the new trace shell exists in `public.traces` with `engine_projection_state = "up_to_date"`
- **AND** the new root span exists in `public.spans`

#### Scenario: Continuation shell uses continuation input
- **WHEN** run N continues as run N+1 with continuation input X
- **THEN** the new trace shell input is X
- **AND** the new root span input is X
- **AND** the old trace input is not copied onto the new shell

#### Scenario: Missing previous trace shell fails continuation
- **WHEN** run N attempts to continue as run N+1
- **AND** the previous projected trace shell row is unexpectedly missing
- **THEN** continuation fails as an invariant violation
- **AND** the engine does not silently create a degraded fallback shell

### Requirement: Signal Wait State Does Not Strand on Continuation
When a run that is waiting on a signal transitions to `continued_as_new`, terminal projection SHALL clear the projected signal wait state on the old trace so the debugger does not retain a stale pending signal wait.

A pure signal wait does not require a synthetic resolved-signal timeline event.

#### Scenario: Signal wait clears on continuation
- **WHEN** a run in a signal wait transitions to `continued_as_new`
- **THEN** the projected `engine_wait_state` is cleared on the old trace
- **AND** the debugger does not retain a stale pending signal wait for that run

### Requirement: Activity and Timer Wait Cleanup Is Explicit on Continuation
When a run transitions to `continued_as_new`, terminal projection SHALL use a continuation-specific cleanup reason for cancelled activity waits and discarded timer waits on the old trace.

Those waits SHALL emit resolved wait events using that continuation cleanup reason.

#### Scenario: Activity and timer waits resolve on continuation
- **WHEN** a run with open activity and timer waits transitions to `continued_as_new`
- **THEN** the old trace emits resolved wait events for those waits
- **AND** the resolution value identifies continuation cleanup rather than cancellation or termination
