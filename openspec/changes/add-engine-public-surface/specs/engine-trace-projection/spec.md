# Capability: engine-trace-projection

Trace linkage columns, projection state machine, deterministic identifiers, and search parity for engine-backed traces projected into the platform data model.

Related capabilities: [engine-projector-runtime](../engine-projector-runtime/spec.md), [engine-public-api](../engine-public-api/spec.md)

## ADDED Requirements

### Requirement: Trace linkage columns

The platform `traces` table MUST include columns that link projected traces to their engine run and track projection freshness.

#### Scenario: Engine linkage columns exist
- **WHEN** the trace linkage migration is applied
- **THEN** the `traces` table gains the following nullable columns:
  - `engine_run_id` (UUID) with a unique index
  - `engine_definition_name` (TEXT)
  - `engine_definition_version` (TEXT)
  - `engine_projection_state` (TEXT)
  - `engine_latest_history_id` (BIGINT)
  - `engine_last_projected_history_id` (BIGINT)
  - `engine_projection_updated_at` (TIMESTAMPTZ)

#### Scenario: Non-engine traces unaffected
- **WHEN** a trace is created through the existing ingest path
- **THEN** all engine linkage columns remain NULL
- **THEN** existing queries, read paths, and API responses are unchanged

#### Scenario: Engine run ID uniqueness
- **WHEN** a projected trace is created for an engine run
- **THEN** the `engine_run_id` unique index prevents duplicate traces for the same run

---

### Requirement: Projection state machine

Each engine-backed trace MUST track its projection freshness through a four-state machine stored in `engine_projection_state`.

#### Scenario: State definitions
- **WHEN** a trace has `engine_projection_state` set
- **THEN** the value is one of: `up_to_date`, `catching_up`, `summary_only`, `journal_expired`

#### Scenario: Initial state on start
- **WHEN** the start handler creates an engine run and its projected trace shell in one transaction
- **THEN** `engine_latest_history_id` and `engine_last_projected_history_id` are both set to the `workflow.started` history event ID
- **THEN** `engine_projection_state` is set to `up_to_date`

#### Scenario: Transition to catching_up
- **WHEN** an activation appends history events that advance `engine_latest_history_id` beyond `engine_last_projected_history_id`
- **THEN** `engine_projection_state` transitions to `catching_up`

#### Scenario: Transition back to up_to_date
- **WHEN** the async projector advances `engine_last_projected_history_id` to equal `engine_latest_history_id`
- **THEN** `engine_projection_state` transitions to `up_to_date`

#### Scenario: Retention cleanup transitions
- **WHEN** retention cleanup deletes projected detail rows from a terminal trace but raw engine history remains
- **THEN** `engine_projection_state` transitions to `summary_only`
- **WHEN** raw engine history is also no longer available
- **THEN** `engine_projection_state` transitions to `journal_expired`

#### Scenario: Per-trace freshness
- **WHEN** the read path checks projection freshness
- **THEN** it uses the stored per-trace `engine_latest_history_id` and `engine_last_projected_history_id` directly
- **THEN** it does not infer freshness from a project-wide checkpoint alone

---

### Requirement: Deterministic projected identifiers

Engine-backed traces, spans, and sessions MUST use formulaic external IDs derived from engine state, not random UUIDs.

#### Scenario: Projected trace ID
- **WHEN** a trace is projected for an engine run
- **THEN** `traces.trace_id` is set to `"engine:" + run_id`

#### Scenario: Projected root span ID
- **WHEN** a root span shell is projected for an engine run
- **THEN** `spans.span_id` is set to `"engine:root:" + run_id`

#### Scenario: Projected activity span ID
- **WHEN** an activity span is projected in Phase 12
- **THEN** `spans.span_id` is set to `"engine:activity:" + run_id + ":" + activity_key`
- **THEN** repeated retries or claims for the same `activity_key` update the same projected span

#### Scenario: Projected session external ID with explicit key
- **WHEN** the start request includes a `session.key`
- **THEN** `sessions.external_id` is set to the provided `session.key`

#### Scenario: Projected session external ID without explicit key
- **WHEN** the start request does not include a `session.key`
- **THEN** `sessions.external_id` is set to the `instance_key`

---

### Requirement: Projected span kind and naming contract

Engine-backed projected spans MUST use kinds and names that fit the current debugger’s existing span taxonomy.

#### Scenario: Root workflow span shape
- **WHEN** the start handler creates the projected root span shell
- **THEN** the span `kind` is `CHAIN`
- **THEN** the span `name` matches the projected trace name

#### Scenario: Activity span shape
- **WHEN** the projector creates or updates a projected activity span
- **THEN** the span `kind` is `TOOL`
- **THEN** the span `name` is `activity_type`

#### Scenario: Attempt-specific spans are deferred
- **WHEN** the current engine history/schema model does not expose a stable attempt identity at `activity.scheduled` time
- **THEN** Phase 12 does not mint `:attempt`-suffixed span IDs
- **THEN** future attempt-specific spans require a prior engine history/schema change

---

### Requirement: Projected trace naming

Engine-backed traces MUST derive their `name` from the start request or the definition name.

#### Scenario: Explicit trace name
- **WHEN** the start request includes `trace.name`
- **THEN** the projected trace `name` is set to the provided value

#### Scenario: Default trace name
- **WHEN** the start request does not include `trace.name`
- **THEN** the projected trace `name` is set to `definition_name`

---

### Requirement: Search parity

Engine-backed traces MUST participate in existing trace search through the same generated search vectors used by ingest traces.

#### Scenario: Projected trace is searchable by name
- **WHEN** an engine-backed trace is projected with a name
- **THEN** the trace's `search_vector` column includes the trace name
- **THEN** the trace appears in search results when queried by name

#### Scenario: Projected trace is searchable by user ID
- **WHEN** an engine-backed trace is projected with a `user_id` from `trace.user_id`
- **THEN** the trace appears in search results when filtered by that user ID

#### Scenario: Activity span names are searchable
- **WHEN** activity spans are projected with names
- **THEN** the span `search_vector` columns include activity span names
