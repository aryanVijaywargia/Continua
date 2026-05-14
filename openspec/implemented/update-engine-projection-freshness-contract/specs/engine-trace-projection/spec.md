## MODIFIED Requirements

### Requirement: Projection state machine
Each engine-backed trace MUST track its projection freshness through a four-state machine stored in `engine_projection_state`, with stored checkpoint columns on `public.traces` remaining the steady-state source of truth and a narrow exception for engine-only terminal cancel/terminate transactions introduced by operational hardening.

#### Scenario: State definitions
- **WHEN** a trace has `engine_projection_state` set
- **THEN** the value is one of: `up_to_date`, `catching_up`, `summary_only`, `journal_expired`

#### Scenario: Initial state on start
- **WHEN** the start handler creates an engine run and its projected trace shell in one transaction
- **THEN** `engine_latest_history_id` and `engine_last_projected_history_id` are both set to the `workflow.started` history event ID
- **THEN** `engine_projection_state` is set to `up_to_date`

#### Scenario: Transition to catching_up on non-terminal activation progress
- **WHEN** a non-terminal activation appends history events that advance `engine_latest_history_id` beyond `engine_last_projected_history_id`
- **THEN** activation updates the stored `engine_latest_history_id`
- **THEN** `engine_projection_state` transitions to `catching_up`

#### Scenario: Terminal cancel and terminate do not advance stored freshness in-band
- **WHEN** `decisionCancelled` or the root-side terminate handler appends terminal history
- **THEN** those transactions do NOT write `public.traces`
- **THEN** the stored `engine_latest_history_id` and `engine_projection_state` MAY remain behind until the projector catches up
- **THEN** this exception exists to preserve the single-writer projection invariant defined by operational hardening

#### Scenario: Transition back to up_to_date
- **WHEN** the async projector advances `engine_last_projected_history_id` to equal `engine_latest_history_id`
- **THEN** `engine_projection_state` transitions to `up_to_date`

#### Scenario: Retention cleanup transitions
- **WHEN** retention cleanup deletes projected detail rows from a terminal trace but raw engine history remains
- **THEN** `engine_projection_state` transitions to `summary_only`
- **WHEN** raw engine history is also no longer available
- **THEN** `engine_projection_state` transitions to `journal_expired`

#### Scenario: Per-trace frontier check
- **WHEN** a read path checks projection freshness
- **THEN** it uses the trace's stored `engine_last_projected_history_id` together with the run's live history frontier
- **THEN** it does not infer freshness from a project-wide checkpoint alone

#### Scenario: Effective catching_up for stale stored shells
- **WHEN** a read surface sees stored `engine_projection_state = up_to_date` but the run's live history frontier is ahead of `engine_last_projected_history_id`
- **THEN** the surface MAY expose an effective `catching_up` state until the projector catches up
- **THEN** the read path does not rewrite `public.traces` just to normalize freshness

#### Scenario: Journal expired remains shell-only
- **WHEN** stored `engine_projection_state = journal_expired`
- **THEN** read paths do not perform live summary supplementation solely to normalize freshness
- **THEN** the retained summary shell remains the only source for that trace
