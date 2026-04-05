# Capability: engine-history-events

Operational hardening additions to the public engine history contract: introduces `workflow.cancelled` and `workflow.terminated` as first-class event types with distinct payloads and replay semantics, so history consumers can distinguish cancelled and terminated outcomes from generic failures.

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-trace-projection](../engine-trace-projection/spec.md)

## ADDED Requirements

### Requirement: workflow.cancelled event type

The history package MUST define `workflow.cancelled` as a canonical terminal event type with an empty payload, distinct from `workflow.failed`.

#### Scenario: Event type constant is registered
- **WHEN** the history event type registry is loaded
- **THEN** it includes `workflow.cancelled` as a canonical event type constant
- **THEN** the constant is exported from the public engine history package

#### Scenario: Payload struct is empty
- **WHEN** a `workflow.cancelled` event is created
- **THEN** its payload struct has no required fields
- **THEN** the JSON payload serializes as an empty object `{}`

#### Scenario: Decode surface recognizes workflow.cancelled
- **WHEN** a history reader decodes a row with event type `workflow.cancelled`
- **THEN** `DecodePayload` returns the empty cancelled payload struct without error
- **THEN** no "unknown event type" error is produced

#### Scenario: workflow.cancelled is terminal
- **WHEN** activation appends `workflow.cancelled`
- **THEN** the run transitions to terminal status `cancelled` in the same transaction
- **THEN** no further history rows for that run are appended by activation

#### Scenario: workflow.cancelled is replay-driving
- **WHEN** replay encounters a recorded `workflow.cancelled` row
- **THEN** replay treats it as the terminal decision and does not re-execute workflow code past the cancellation point
- **THEN** any trailing events after `workflow.cancelled` are treated as a replay mismatch

---

### Requirement: workflow.terminated event type

The history package MUST define `workflow.terminated` as a canonical terminal event type with an operator-facing error payload, distinct from `workflow.failed` and `workflow.cancelled`.

#### Scenario: Event type constant is registered
- **WHEN** the history event type registry is loaded
- **THEN** it includes `workflow.terminated` as a canonical event type constant
- **THEN** the constant is exported from the public engine history package

#### Scenario: Payload struct carries error fields
- **WHEN** a `workflow.terminated` event is created
- **THEN** its payload struct includes `error_code` and `error_message` fields with canonical JSON tag names
- **THEN** the struct is exported from the public engine history package

#### Scenario: Canonical payload values for operator terminate
- **WHEN** the platform terminate handler appends `workflow.terminated`
- **THEN** the payload has `error_code="terminated"` and `error_message="run terminated by operator"`

#### Scenario: Decode surface recognizes workflow.terminated
- **WHEN** a history reader decodes a row with event type `workflow.terminated`
- **THEN** `DecodePayload` returns the typed terminated payload struct without error
- **THEN** the returned struct contains the original `error_code` and `error_message` values

#### Scenario: workflow.terminated is terminal
- **WHEN** the terminate handler appends `workflow.terminated`
- **THEN** the run transitions to terminal status `terminated` in the same transaction
- **THEN** no further history rows for that run are appended after termination

#### Scenario: workflow.terminated is not replay-driving
- **WHEN** a terminated run exists
- **THEN** the engine does not reactivate or replay the run
- **THEN** `workflow.terminated` is decode-supported for history and projector consumers but never consulted by an activation transaction

---

### Requirement: History output includes cancelled and terminated

The history reader API surfaces (engine history endpoint, projector journal reads) MUST include `workflow.cancelled` and `workflow.terminated` as fully-typed events.

#### Scenario: History endpoint returns cancelled event
- **WHEN** a history reader fetches events for a run that ended with `CANCELLED`
- **THEN** the response contains a `workflow.cancelled` event as the terminal row
- **THEN** the event appears after any activity, timer, signal, or cancel events that preceded it

#### Scenario: History endpoint returns terminated event
- **WHEN** a history reader fetches events for a run that ended with `TERMINATED`
- **THEN** the response contains a `workflow.terminated` event as the terminal row
- **THEN** the event payload carries `error_code` and `error_message`

#### Scenario: Projector journal consumes both events
- **WHEN** the projector catches up on history for a terminal run
- **THEN** it reads `workflow.cancelled` or `workflow.terminated` via the same decode path as other events
- **THEN** it maps them to the appropriate projected engine summary status

---

### Requirement: Distinction between cancelled, terminated, and failed

History consumers MUST be able to differentiate `workflow.cancelled`, `workflow.terminated`, and `workflow.failed` outcomes by event type alone, without parsing error codes.

#### Scenario: Generic failure remains workflow.failed
- **WHEN** a workflow returns a non-nil error that is NOT `workflow.ErrCancelled`
- **THEN** replay produces `workflow.failed` with the error code and message from the returned error
- **THEN** the event type is `workflow.failed`, not `workflow.cancelled`

#### Scenario: Cancelled is never encoded as workflow.failed
- **WHEN** a workflow returns `workflow.ErrCancelled`
- **THEN** the terminal history event type is `workflow.cancelled`
- **THEN** no `workflow.failed` row is appended for the same run

#### Scenario: Terminated is never encoded as workflow.failed
- **WHEN** the operator terminate handler stops a run
- **THEN** the terminal history event type is `workflow.terminated`
- **THEN** no `workflow.failed` row is appended for the same run
