# Capability: engine-debugger-integration

Extended read schemas and debugger UI surfaces for displaying engine-backed traces alongside traditional ingest traces in the existing trace, session, and compare flows.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-projector-runtime](../engine-projector-runtime/spec.md)

## ADDED Requirements

### Requirement: Trace engine metadata in read schemas

Existing trace read schemas MUST extend with a nested `engine` object where engine metadata materially helps the debugger.

#### Scenario: Trace list engine metadata
- **WHEN** a trace list response includes an engine-backed trace
- **THEN** the `Trace` schema includes an optional `engine` object with: `run_id`, `definition_name`, `definition_version`, `projection_state`

#### Scenario: Trace detail engine metadata
- **WHEN** a trace detail response is returned for an engine-backed trace
- **THEN** the `TraceDetail` schema includes an `engine` object with: full engine summary, wait state, pending work counts, custom status, and terminal result/failure summary

#### Scenario: Non-engine traces have null engine field
- **WHEN** a trace list or detail response includes a non-engine trace
- **THEN** the `engine` field is `null` or absent

---

### Requirement: Compare trace header engine metadata

The compare trace header schema MUST include engine metadata when present.

#### Scenario: Compare header with engine trace
- **WHEN** a compare flow includes an engine-backed trace
- **THEN** the `CompareTraceHeader` schema includes an optional `engine` object with: `run_id`, `definition_name`, `definition_version`, `projection_state`

#### Scenario: Compare header without engine trace
- **WHEN** a compare flow includes a non-engine trace
- **THEN** the `engine` field is `null` or absent

---

### Requirement: Timeline engine metadata

The timeline response MUST include projection-state metadata for engine-backed traces.

#### Scenario: Timeline with engine trace
- **WHEN** the timeline response is returned for an engine-backed trace
- **THEN** `TimelineResponse` includes an optional `engine` object with projection-state metadata only

#### Scenario: Timeline with non-engine trace
- **WHEN** the timeline response is returned for a non-engine trace
- **THEN** the `engine` field is `null` or absent

---

### Requirement: Trace detail engine fallback read

The trace detail read path MUST supplement projected data with live engine summary when projection is not current.

#### Scenario: Projection up to date
- **WHEN** an engine-backed trace has `engine_projection_state = up_to_date`
- **THEN** the trace detail is served entirely from projected platform rows
- **THEN** no engine-side read is performed

#### Scenario: Projection catching up
- **WHEN** an engine-backed trace has `engine_projection_state = catching_up`
- **THEN** the trace detail read path MUST call a root-side engine-read helper keyed by `traces.engine_run_id`
- **THEN** the helper supplements projected data with current engine run summary (status, custom status, wait state, pending counts)

#### Scenario: Summary only fallback
- **WHEN** an engine-backed trace has `engine_projection_state = summary_only`
- **THEN** the trace detail read path MUST call the root-side engine-read helper to supplement the retained summary shell with current engine run state
- **THEN** span and event detail is not available (projected detail rows were cleaned up)

#### Scenario: Journal expired fallback
- **WHEN** an engine-backed trace has `engine_projection_state = journal_expired`
- **THEN** the trace detail is served from the retained summary shell only
- **THEN** no engine-side read is performed (raw history is no longer available)

#### Scenario: No cross-schema joins in list queries
- **WHEN** the trace list query is executed
- **THEN** no cross-schema joins to engine tables are added to the existing list query

---

### Requirement: Trace list engine badges

The trace list UI MUST display engine badges on engine-backed traces.

#### Scenario: Engine badge displayed
- **WHEN** a trace in the trace list has a non-null `engine` object
- **THEN** the UI displays an engine badge indicating this is an engine-backed trace

#### Scenario: Non-engine traces unchanged
- **WHEN** a trace in the trace list has no `engine` object
- **THEN** the UI renders the trace identically to before this change

---

### Requirement: Trace detail engine surfaces

The trace detail UI MUST display engine wait-state summary and projection-state banners for engine-backed traces.

#### Scenario: Wait-state summary
- **WHEN** the trace detail is displayed for an engine-backed trace with a `waiting` run status
- **THEN** the UI shows a wait-state summary indicating what the workflow is waiting for (activity, timer, or signal)

#### Scenario: Signal waits remain visible without projected timeline wait rows
- **WHEN** an engine-backed trace is waiting on a signal and Phase 12 has no contract-complete projected signal `wait` event pair
- **THEN** the UI still shows the current wait through `TraceDetail.engine.wait_state`
- **THEN** the missing timeline `wait` row does not hide the live waiting reason

#### Scenario: Projection-state banner for catching_up
- **WHEN** the trace detail is displayed for an engine-backed trace with `engine_projection_state = catching_up`
- **THEN** the UI shows a banner indicating that projected detail may not yet reflect the latest engine state

#### Scenario: Projection-state banner for summary_only
- **WHEN** the trace detail is displayed for an engine-backed trace with `engine_projection_state = summary_only`
- **THEN** the UI shows a banner indicating that detailed span and event data has been cleaned up but summary information remains

#### Scenario: Non-engine trace detail unchanged
- **WHEN** the trace detail is displayed for a non-engine trace
- **THEN** no engine-related banners or summaries are shown

---

### Requirement: Session detail engine traces

Engine-backed traces MUST appear in session detail through the existing trace table and compare flow.

#### Scenario: Engine traces in session trace list
- **WHEN** a session detail page lists traces for a session
- **THEN** engine-backed traces associated with that session appear in the trace list with engine badges

#### Scenario: Engine traces in compare flow
- **WHEN** two traces are compared in a session and one or both are engine-backed
- **THEN** compare headers show engine metadata when present
- **THEN** projected engine semantic events use the existing compare keys (`question`, `effect_id`, `wait_id`) so the compare flow works without an engine-specific pairing path

---

### Requirement: Session-level engine surfaces deferred

Session-level engine aggregate badges and `Session.engine` metadata MUST NOT be added in Phase 12.

#### Scenario: No Session.engine field
- **WHEN** a session list or detail response is returned
- **THEN** no `engine` field is present on the session schema

#### Scenario: No session-list engine badges
- **WHEN** the session list UI is rendered
- **THEN** no engine aggregate badges are displayed at the session level
