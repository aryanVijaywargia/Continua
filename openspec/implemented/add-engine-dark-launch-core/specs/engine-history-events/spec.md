# Capability: engine-history-events

Canonical event types, payload structs, and JSON encoding for the engine history table. Lives in `engine/internal/history`.

Related capabilities: [engine-workflow-authoring](../engine-workflow-authoring/spec.md), [engine-runtime-execution](../engine-runtime-execution/spec.md)

## ADDED Requirements

### Requirement: Canonical event type registry

The history package MUST define canonical event type constants for all runtime events.

#### Scenario: Workflow lifecycle events
- **WHEN** the event type registry is defined
- **THEN** it includes: `workflow.started`, `workflow.completed`, `workflow.failed`, `workflow.replay_mismatch`

#### Scenario: Activity events
- **WHEN** the event type registry is defined
- **THEN** it includes: `activity.scheduled`, `activity.completed`, `activity.failed`

#### Scenario: Timer events
- **WHEN** the event type registry is defined
- **THEN** it includes: `timer.scheduled`, `timer.fired`

#### Scenario: Signal and cancel events
- **WHEN** the event type registry is defined
- **THEN** it includes: `signal.received`, `cancel.requested`

#### Scenario: Status events
- **WHEN** the event type registry is defined
- **THEN** it includes: `custom_status.updated`

---

### Requirement: Typed payload structs

Each event type MUST have a corresponding Go struct for its payload, with canonical JSON field names.

#### Scenario: Workflow started payload
- **WHEN** a `workflow.started` event is created
- **THEN** its payload includes: `definition_name`, `definition_version`, `instance_key`, `input` (optional)

#### Scenario: Workflow started is the initial run event
- **WHEN** `start` creates a new run
- **THEN** it appends `workflow.started` as the first history row for that run
- **THEN** the payload carries the durable workflow input contract for later replay via `Context.Input(...)`

#### Scenario: Activity scheduled payload
- **WHEN** an `activity.scheduled` event is created
- **THEN** its payload includes: `activity_key`, `activity_type`, `input`

#### Scenario: Activity completed payload
- **WHEN** an `activity.completed` event is created
- **THEN** its payload includes: `activity_key`, `activity_type`, `output`

#### Scenario: Activity failed payload
- **WHEN** an `activity.failed` event is created
- **THEN** its payload includes: `activity_key`, `activity_type`, `error_code`, `error_message`

#### Scenario: Timer scheduled payload
- **WHEN** a `timer.scheduled` event is created
- **THEN** its payload includes: `timer_key`, `due_at` (RFC 3339)

#### Scenario: Timer fired payload
- **WHEN** a `timer.fired` event is created
- **THEN** its payload includes: `timer_key`

#### Scenario: Signal received payload
- **WHEN** a `signal.received` event is created
- **THEN** its payload includes: `signal_name`, `payload` (the signal data)

#### Scenario: Cancel requested payload
- **WHEN** a `cancel.requested` event is created
- **THEN** its payload is empty or minimal (no additional data required)

#### Scenario: Workflow completed payload
- **WHEN** a `workflow.completed` event is created
- **THEN** its payload includes: `result`

#### Scenario: Workflow failed payload
- **WHEN** a `workflow.failed` event is created
- **THEN** its payload includes: `error_code`, `error_message`

#### Scenario: Replay mismatch payload
- **WHEN** a `workflow.replay_mismatch` event is created
- **THEN** its payload includes: `expected_type`, `expected_key`, `actual_type`, `actual_key`, `detail`

#### Scenario: Custom status updated payload
- **WHEN** a `custom_status.updated` event is created
- **THEN** its payload includes: `status` (the custom status value)

---

### Requirement: Structured replay comparison

Event payloads MUST support structured comparison for replay validation. Phase 11 uses typed Go structs for all events, so comparison operates on deserialized struct fields rather than raw JSON bytes.

#### Scenario: Typed struct comparison
- **WHEN** the replay engine compares a new event against a recorded history event
- **THEN** comparison deserializes both payloads into the corresponding typed Go struct and compares primitive kind, stable key, and struct field values

#### Scenario: Failed activity outcomes participate in replay
- **WHEN** replay encounters an `activity.failed` history event for a workflow `Activity(...)` call
- **THEN** it uses the recorded `activity_key`, `activity_type`, `error_code`, and `error_message` fields to deterministically return the same failure without re-executing the handler

#### Scenario: Standard JSON encoding
- **WHEN** an event payload is serialized to JSON
- **THEN** standard `encoding/json` marshaling is used (no custom canonicalizer required in Phase 11 because all payloads are typed structs with deterministic field ordering)
