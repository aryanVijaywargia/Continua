# Capability: engine-projector-runtime

Async projector loop in `continua-engine serve`, writer ownership contract, projection event mapping, and event taxonomy rules for projecting engine history into platform tables.

Related capabilities: [engine-trace-projection](../engine-trace-projection/spec.md), [engine-debugger-integration](../engine-debugger-integration/spec.md)

## ADDED Requirements

### Requirement: Projector loop

The `continua-engine serve` command MUST run a fourth polling loop: the projector.

#### Scenario: Projector starts with serve
- **WHEN** `continua-engine serve` starts
- **THEN** a projector loop runs alongside the existing workflow, activity, and maintenance loops
- **THEN** the projector polls for engine-backed traces where `engine_last_projected_history_id < engine_latest_history_id`

#### Scenario: Projector checkpoint monotonicity
- **WHEN** the projector advances `engine_last_projected_history_id` for a trace
- **THEN** the new value is strictly greater than the previous value
- **THEN** the projector never regresses a checkpoint

#### Scenario: Projector restart safety
- **WHEN** the projector restarts after a crash
- **THEN** it resumes from each trace's stored `engine_last_projected_history_id`
- **THEN** previously projected rows are idempotent (deterministic IDs prevent duplicates)

#### Scenario: Projected span events are replay-safe
- **WHEN** the projector emits one or more `public.span_events` rows from a single engine history row
- **THEN** each projected event uses a deterministic `idempotency_key` derived from `(run_id, history_id, projection_variant)`
- **THEN** rerunning the projector does not duplicate semantic or custom events

---

### Requirement: Writer ownership

Each writer that touches `public.traces` for an engine-backed trace MUST respect strict ownership boundaries across the three permitted writers.

#### Scenario: Start handler ownership
- **WHEN** the start handler creates an engine run
- **THEN** it creates the projected trace, root span shell, and session in one transaction
- **THEN** it owns initial metadata seeding: name, user_id, tags, environment, release, metadata, status, start_time
- **THEN** it sets `engine_projection_state` to `up_to_date`

#### Scenario: Terminal activation ownership
- **WHEN** an activation transaction completes, fails, or cancels a run
- **THEN** within the same activation transaction, it writes terminal summary fields on the projected trace and root span
- **THEN** it is authoritative for: `traces.status`, `traces.end_time`, terminal output/failure summary on the root span

#### Scenario: Async projector ownership
- **WHEN** the async projector processes new history events for a trace
- **THEN** it creates per-activity spans, projected semantic events in `span_events`, and updates counters
- **THEN** it advances `engine_last_projected_history_id`
- **THEN** it does not own terminal summary fields

---

### Requirement: Two-writer ordering contract

The async projector MUST NOT overwrite terminal summary fields set by the terminal activation writer.

#### Scenario: Terminal trace protection
- **WHEN** an engine-backed trace has a terminal status (`completed`, `failed`, `cancelled`)
- **THEN** projector SQL uses guards that prevent overwriting `traces.status`, `traces.end_time`, or terminal output/failure summary
- **THEN** the projector only advances detail/counter fields on terminal traces

#### Scenario: Non-terminal trace projection
- **WHEN** an engine-backed trace has a non-terminal status (`running`)
- **THEN** the projector may update status-adjacent fields (like span counts) without restriction

#### Scenario: Writer ordering regression test
- **WHEN** a terminal activation writes summary fields
- **THEN** a subsequent projector run for the same trace does not regress any summary field values

---

### Requirement: Projection event mapping

The projector MUST map engine history events to platform span and event rows following a defined mapping.

#### Scenario: Workflow started is owned by start handler
- **WHEN** the start handler creates projected shells
- **THEN** it sets `engine_last_projected_history_id` to the `workflow.started` event ID
- **THEN** the projector never sees the `workflow.started` event because the checkpoint starts past it
- **THEN** the start handler is the sole owner of initial trace shell metadata

#### Scenario: Activity events create spans
- **WHEN** the projector processes `activity.scheduled`, `activity.completed`, or `activity.failed` history events
- **THEN** it creates or updates per-activity spans in `public.spans` using deterministic span IDs
- **THEN** projected activity spans use `kind = TOOL` and `name = activity_type`
- **THEN** `activity.scheduled` creates a new span with status `STARTED`
- **THEN** `activity.completed` sets the span status to `COMPLETED` with output
- **THEN** `activity.failed` sets the span status to `FAILED` with error details

#### Scenario: Activity schedule projects effect and wait-entered semantics
- **WHEN** the projector processes `activity.scheduled`
- **THEN** it emits an `effect` event on the root span with payload:
  - `effect_kind = "activity"`
  - `has_external_side_effect = true`
  - `effect_id = "activity:" + activity_key`
- **THEN** it emits a `wait` event on the root span with payload:
  - `wait_kind = "activity"`
  - `phase = "entered"`
  - `wait_id = "activity:" + activity_key`

#### Scenario: Activity terminal outcome projects decision and wait-resolved semantics
- **WHEN** the projector processes `activity.completed` or `activity.failed`
- **THEN** it emits a `decision` event on the root span with payload:
  - `question = "activity:" + activity_key + ":outcome"`
  - `chosen = "completed"` or `"failed"`
  - `reasoning = error_message` only for failed activity outcomes
- **THEN** it emits a `wait` event on the root span with payload:
  - `wait_kind = "activity"`
  - `phase = "resolved"`
  - `wait_id = "activity:" + activity_key`
  - `resolution = "completed"` or `"failed"`

#### Scenario: Timer lifecycle projects wait-entered and wait-resolved semantics
- **WHEN** the projector processes `timer.scheduled`
- **THEN** it emits a `wait` event on the root span with payload:
  - `wait_kind = "timer"`
  - `phase = "entered"`
  - `wait_id = "timer:" + timer_key`
- **WHEN** the projector processes `timer.fired`
- **THEN** it emits a `wait` event on the root span with payload:
  - `wait_kind = "timer"`
  - `phase = "resolved"`
  - `wait_id = "timer:" + timer_key`
  - `resolution = "fired"`

#### Scenario: One history row may emit multiple semantic events
- **WHEN** a single engine history row needs to satisfy the existing debugger contract in more than one semantic dimension
- **THEN** the projector may emit multiple `public.span_events` rows for that row (for example `activity.scheduled` => `effect` plus `wait`, `activity.failed` => `decision` plus `wait`)

#### Scenario: Only contract-complete semantics are projected
- **WHEN** the projector processes `signal.received`, `cancel.requested`, `custom_status.updated`, or any other history row that does not provide a full current `decision` / `effect` / `wait` payload contract
- **THEN** it emits a `custom` event with the original engine event type preserved in payload metadata
- **THEN** Phase 12 does not synthesize partial semantic wait rows for signal waits

---

### Requirement: Event taxonomy contract

The projector MUST only emit event types already supported by the current platform read path.

#### Scenario: Supported event types only
- **WHEN** the projector creates span events in `public.span_events`
- **THEN** it uses only these event types: `decision`, `effect`, `wait`, and existing `custom`

#### Scenario: Non-semantic history rows use custom type
- **WHEN** an engine history event does not map to a semantic event type (`decision`, `effect`, `wait`)
- **THEN** it is projected as a `custom` event with the original engine event type preserved in the payload metadata

#### Scenario: No new event types in Phase 12
- **WHEN** the projector creates events
- **THEN** no new event types (such as `engine_custom`) are introduced

#### Scenario: Semantic payload keys match current debugger expectations
- **WHEN** the projector emits `decision`, `effect`, or `wait` events
- **THEN** decision payloads include a stable `question`
- **THEN** effect payloads include a stable `effect_id`
- **THEN** wait payloads include stable `wait_id`, `phase`, and `resolution` values where applicable

---

### Requirement: Cross-schema SQL isolation

All handwritten cross-schema SQL used by the projector MUST be isolated in one engine-local package or file group.

#### Scenario: Projection SQL locality
- **WHEN** the projector writes to `public.*` tables
- **THEN** all cross-schema SQL is contained in a single engine-local projection package or file group
- **THEN** each file includes header comments citing the authoritative platform schema and query inputs the SQL depends on

#### Scenario: Platform schema dependency documentation
- **WHEN** projector SQL references `public.traces`, `public.spans`, or `public.span_events` columns
- **THEN** the SQL file headers cite the platform migration version and column names they depend on
