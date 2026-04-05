## MODIFIED Requirements

### Requirement: Trace detail engine fallback read
The trace detail read path MUST determine freshness from the effective per-trace projection state, not only the raw stored enum value, so engine-only terminal cancel/terminate transactions can remain projection-write-free without surfacing stale `up_to_date` detail.

#### Scenario: Effective projection up to date
- **WHEN** an engine-backed trace has stored `engine_projection_state = up_to_date`
- **AND** the run's live history frontier is not ahead of `engine_last_projected_history_id`
- **THEN** the trace detail is served entirely from projected platform rows
- **THEN** no engine-side read is performed

#### Scenario: Projection catching up from stored state
- **WHEN** an engine-backed trace has stored `engine_projection_state = catching_up`
- **THEN** the trace detail read path MUST call a root-side engine-read helper keyed by `traces.engine_run_id`
- **THEN** the helper supplements projected data with current engine run summary (status, custom status, wait state, pending counts)

#### Scenario: Projection catching up from stale stored shell
- **WHEN** an engine-backed trace has stored `engine_projection_state = up_to_date`
- **AND** the run's live history frontier is ahead of `engine_last_projected_history_id`
- **THEN** the trace detail read path treats the trace as effectively `catching_up`
- **THEN** the helper supplements projected data with current engine run summary (status, custom status, wait state, pending counts)
- **THEN** the read path does not rewrite `public.traces`

#### Scenario: Summary only fallback
- **WHEN** an engine-backed trace has `engine_projection_state = summary_only`
- **THEN** the trace detail read path MUST call the root-side engine-read helper to supplement the retained summary shell with current engine run state
- **THEN** span and event detail is not available (projected detail rows were cleaned up)

#### Scenario: Journal expired fallback
- **WHEN** an engine-backed trace has `engine_projection_state = journal_expired`
- **THEN** the trace detail is served from the retained summary shell only
- **THEN** no engine-side read is performed (raw history is no longer available)

## ADDED Requirements

### Requirement: Read metadata exposes effective projection state
Debugger read surfaces that expose engine projection state MUST report the effective per-trace freshness state rather than blindly echoing a stale stored `up_to_date` shell.

#### Scenario: Trace, timeline, and compare metadata normalize stale up_to_date
- **WHEN** trace detail, trace list, timeline metadata, or compare headers expose `projection_state` for an engine-backed trace
- **AND** the stored state is `up_to_date` but the run's live history frontier is ahead of `engine_last_projected_history_id`
- **THEN** those surfaces expose `catching_up`
- **THEN** this normalization does not write back to `public.traces`

#### Scenario: Journal expired metadata remains unchanged
- **WHEN** a read surface exposes `projection_state` for a trace whose stored state is `journal_expired`
- **THEN** it continues to expose `journal_expired`
- **THEN** it does not downgrade or supplement the trace solely to normalize freshness
