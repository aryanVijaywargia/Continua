## MODIFIED Requirements

### Requirement: Ingest Event Type Enum Extension
The `IngestEventType` OpenAPI enum SHALL include `state_change` and `decision` in addition to the existing values (`log`, `error`, `exception`, `message`, `metric`, `custom`). The `TimelineEventType` enum SHALL include the same two new values alongside existing explicit and synthetic types.

#### Scenario: Ingest a state_change event
- **WHEN** a client sends `POST /v1/ingest` with an event having `event_type: "state_change"`
- **THEN** the event is accepted and stored
- **AND** the event appears in timeline responses with `event_type: "state_change"`

#### Scenario: Ingest a decision event
- **WHEN** a client sends `POST /v1/ingest` with an event having `event_type: "decision"`
- **THEN** the event is accepted and stored
- **AND** the event appears in timeline responses with `event_type: "decision"`

### Requirement: Processor Warning for Missing Semantic Fields
The ingest processor SHALL accept `state_change` and `decision` events regardless of payload content. When a `state_change` event payload is missing the `key` field, the processor SHALL log a warning via `log.Printf("[WARN] ...")`. When a `decision` event payload is missing `question` or `chosen`, the processor SHALL log a warning. The processor SHALL NOT reject the event and SHALL NOT modify the ingest response shape or `ProcessedBatch` return path.

#### Scenario: state_change with complete payload
- **WHEN** a `state_change` event is ingested with payload containing `key`, `old_value`, and `new_value`
- **THEN** the event is accepted without warnings

#### Scenario: state_change with missing key field
- **WHEN** a `state_change` event is ingested with payload missing the `key` field
- **THEN** the event is accepted
- **AND** a `log.Printf("[WARN] ...")` warning is logged server-side

#### Scenario: decision with missing question
- **WHEN** a `decision` event is ingested with payload missing the `question` field
- **THEN** the event is accepted
- **AND** a `log.Printf("[WARN] ...")` warning is logged server-side

### Requirement: Timeline Event Type Preservation
When `state_change` or `decision` events are stored in the database and retrieved for timeline responses, the API SHALL return them with their original event types (`state_change`, `decision`). They SHALL NOT be degraded to `custom` or any other fallback type.

#### Scenario: state_change preserved in timeline response
- **WHEN** a `state_change` event is ingested and later retrieved via the timeline API
- **THEN** the response contains `event_type: "state_change"`, not `"custom"`

#### Scenario: decision preserved in timeline response
- **WHEN** a `decision` event is ingested and later retrieved via the timeline API
- **THEN** the response contains `event_type: "decision"`, not `"custom"`

## ADDED Requirements

### Requirement: Python SDK State Change Helper
The Python SDK `Span` class SHALL provide a `state_change(key, old_value, new_value, namespace=None, message=None)` method that records an event with `event_type: "state_change"` and a payload containing `key`, `old_value`, `new_value`, and optionally `namespace`.

#### Scenario: Record a state change via SDK
- **WHEN** a user calls `span.state_change("user.status", "active", "suspended")`
- **THEN** an event is recorded with `event_type: "state_change"`, `level: "info"`, and payload `{"key": "user.status", "old_value": "active", "new_value": "suspended"}`

#### Scenario: Record a namespaced state change
- **WHEN** a user calls `span.state_change("status", "active", "suspended", namespace="user")`
- **THEN** the payload includes `{"key": "status", "namespace": "user", "old_value": "active", "new_value": "suspended"}`

### Requirement: Python SDK Decision Helper
The Python SDK `Span` class SHALL provide a `decision(question, chosen, alternatives=None, reasoning=None, message=None)` method that records an event with `event_type: "decision"` and a payload containing `question`, `chosen`, and optionally `alternatives` and `reasoning`.

#### Scenario: Record a decision via SDK
- **WHEN** a user calls `span.decision("Which model to use?", "gpt-4")`
- **THEN** an event is recorded with `event_type: "decision"`, `level: "info"`, and payload `{"question": "Which model to use?", "chosen": "gpt-4"}`

#### Scenario: Record a decision with alternatives
- **WHEN** a user calls `span.decision("Which model?", "gpt-4", alternatives=["gpt-3.5", "claude-3"], reasoning="Higher accuracy needed")`
- **THEN** the payload includes `{"question": "Which model?", "chosen": "gpt-4", "alternatives": ["gpt-3.5", "claude-3"], "reasoning": "Higher accuracy needed"}`

### Requirement: Frontend Event Type Updates
The frontend timeline event type definitions SHALL include `"state_change"` and `"decision"` as valid event types. Timeline event summary logic SHALL return meaningful summaries for both new types.

#### Scenario: Summarize a state_change event with payload
- **WHEN** a state_change event has payload `{"key": "user.status", "old_value": "active", "new_value": "suspended"}`
- **THEN** `summarizeTimelineEvent` returns a summary like `"user.status: active → suspended"`

#### Scenario: Summarize a decision event with payload
- **WHEN** a decision event has payload `{"question": "Which model?", "chosen": "gpt-4"}`
- **THEN** `summarizeTimelineEvent` returns a summary like `"Which model? → gpt-4"`

#### Scenario: Summarize event with missing semantic fields
- **WHEN** a state_change or decision event is missing required payload fields
- **AND** the event has a `message` field
- **THEN** `summarizeTimelineEvent` returns the `message` value

### Requirement: Timeline Rendering for New Event Types
The `Timeline` component SHALL render `state_change` events with inline old→new value display when `payload.key` is present, and degrade to generic event row using `message` when `key` is absent. The component SHALL render `decision` events with question and chosen value display when `payload.question` and `payload.chosen` are present, and degrade to generic event row using `message` when either is absent.

#### Scenario: Render state_change with full payload
- **WHEN** a timeline contains a `state_change` event with `payload.key`, `payload.old_value`, `payload.new_value`
- **THEN** the Timeline renders an inline display showing the key and old→new transition

#### Scenario: Render state_change with missing key
- **WHEN** a timeline contains a `state_change` event without `payload.key`
- **THEN** the Timeline renders the event as a generic event row using the `message` field

#### Scenario: Render decision with full payload
- **WHEN** a timeline contains a `decision` event with `payload.question` and `payload.chosen`
- **THEN** the Timeline renders the question and chosen value

#### Scenario: Render decision with missing question
- **WHEN** a timeline contains a `decision` event without `payload.question`
- **THEN** the Timeline renders the event as a generic event row using the `message` field
