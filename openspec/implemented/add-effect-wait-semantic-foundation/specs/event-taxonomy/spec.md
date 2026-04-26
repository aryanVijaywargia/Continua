## ADDED Requirements

### Requirement: Accepted and Preserved Effect Event Type

The system SHALL accept `effect` as a valid ingest event type, persist it, and return it as a recognized timeline event type.

The system SHALL include `effect` in the `IngestEventType` and `TimelineEventType` OpenAPI enums.

When an `effect` event is ingested, the system SHALL store the raw `effect` type in `span_events.event_type` and return it as `event_type: "effect"` on the timeline read path.

_Note: This phase adds wire-level acceptance and preservation only. SDK helpers, documentation conventions, and frontend summarization for `effect` are deferred._

#### Scenario: Ingest accepts effect event type
- **WHEN** a client sends an ingest payload with `event_type: "effect"`
- **THEN** the system SHALL accept the event without validation errors
- **AND** the event SHALL be persisted with `event_type = "effect"` in `span_events`

#### Scenario: Timeline returns effect as recognized type
- **WHEN** a stored event has `event_type = "effect"`
- **THEN** the timeline API SHALL return it with `event_type: "effect"` and `source: "explicit"`

---

### Requirement: Accepted and Preserved Wait Event Type

The system SHALL accept `wait` as a valid ingest event type, persist it, and return it as a recognized timeline event type.

The system SHALL include `wait` in the `IngestEventType` and `TimelineEventType` OpenAPI enums.

When a `wait` event is ingested, the system SHALL store the raw `wait` type in `span_events.event_type` and return it as `event_type: "wait"` on the timeline read path.

_Note: This phase adds wire-level acceptance and preservation only. SDK helpers, documentation conventions, and frontend summarization for `wait` are deferred._

#### Scenario: Ingest accepts wait event type
- **WHEN** a client sends an ingest payload with `event_type: "wait"`
- **THEN** the system SHALL accept the event without validation errors
- **AND** the event SHALL be persisted with `event_type = "wait"` in `span_events`

#### Scenario: Timeline returns wait as recognized type
- **WHEN** a stored event has `event_type = "wait"`
- **THEN** the timeline API SHALL return it with `event_type: "wait"` and `source: "explicit"`

#### Scenario: Orphan wait events are accepted (confirms existing behavior for new type)
- **WHEN** a `wait` event references a `span_id` that has not yet been ingested
- **THEN** the system SHALL store the event successfully
- **AND** the timeline SHALL return the event without a synthesized `span_name`

_Note: Orphan event acceptance is existing behavior (no FK on `span_id`). This scenario confirms the new `wait` type follows the same path._

---

### Requirement: Deterministic Semantic ID Derivation

The system SHALL derive deterministic `effect_id` or `wait_id` values for `effect` and `wait` events when the caller does not provide one.

The derivation SHALL be stateless: identical inputs SHALL always produce identical outputs across service instances and restarts.

Derived IDs SHALL be stored in the event payload and SHALL NOT replace `TimelineEvent.id`, which remains the persisted row UUID.

#### Scenario: Caller-provided effect_id is preserved
- **WHEN** an `effect` event is ingested with a non-empty string `payload.effect_id`
- **THEN** the system SHALL preserve the caller-provided `effect_id` unchanged

#### Scenario: Caller-provided wait_id is preserved
- **WHEN** a `wait` event is ingested with a non-empty string `payload.wait_id`
- **THEN** the system SHALL preserve the caller-provided `wait_id` unchanged

#### Scenario: Deterministic effect_id derived when absent
- **WHEN** an `effect` event is ingested without `payload.effect_id` (or with empty/non-string value)
- **THEN** the system SHALL derive a deterministic `effect_id` in the format `effect_<32hex>`
- **AND** the derived ID SHALL be stored in `payload.effect_id`

#### Scenario: Deterministic wait_id derived when absent
- **WHEN** a `wait` event is ingested without `payload.wait_id` (or with empty/non-string value)
- **THEN** the system SHALL derive a deterministic `wait_id` in the format `wait_<32hex>`
- **AND** the derived ID SHALL be stored in `payload.wait_id`

#### Scenario: Repeated derivation produces identical IDs
- **WHEN** two `effect` events with identical `trace_id`, `span_id`, `sequence`, `event_ts`, `level`, `message`, and `payload` are processed
- **THEN** the system SHALL derive the same `effect_id` for both

#### Scenario: Derivation is stateless (pure function of inputs)
- **WHEN** the same event inputs are processed by separate calls to the derivation function
- **THEN** the derived semantic ID SHALL be identical

_Note: This is tested as a unit property of the derivation helper, not as a distributed systems test. It validates no hidden state (counters, caches) affects output._

#### Scenario: Nil and empty payload produce identical fallback IDs
- **WHEN** an `effect` event has nil payload and another has empty `{}` payload
- **AND** all other tuple fields are identical
- **THEN** the system SHALL derive the same `effect_id` for both

#### Scenario: Nested payload objects are recursively sorted for fallback hashing
- **WHEN** two `effect` events have identical payload content but different JSON key ordering at nested levels
- **AND** both lack `sequence` and `event_ts`
- **THEN** the system SHALL derive the same `effect_id` for both

#### Scenario: Semantic ID derivation uses pre-truncation payload and survives truncation
- **WHEN** an `effect` event is ingested with a large payload that triggers truncation
- **THEN** the `effect_id` SHALL be derived from the original pre-truncation payload content
- **AND** the derived `effect_id` SHALL be present in the persisted (possibly truncated) payload
- **AND** the semantic ID key SHALL NOT be lost due to truncation regardless of payload size

---

### Requirement: Forward-Compatible Unknown Event Type Acceptance

The server-side ingest validation SHALL accept any non-empty explicit event type string, not only the types enumerated in the OpenAPI `IngestEventType` schema.

The system SHALL explicitly reject event type strings that correspond to synthetic timeline-only types: `span_started`, `span_completed`, and `span_failed`.

The synthetic-type blocklist SHALL be maintained in a single helper or set with a code comment noting its coupling to the OpenAPI contract and timeline code.

The OpenAPI `IngestEventType` enum SHALL remain strict (not widened to free-form strings).

#### Scenario: Known explicit types are accepted
- **WHEN** a client sends an event with a known `event_type` such as `"log"`, `"effect"`, or `"custom"`
- **THEN** the system SHALL accept the event

#### Scenario: Unknown explicit types are accepted server-side
- **WHEN** a client sends an event with `event_type: "workflow_step"` (not in the OpenAPI enum)
- **THEN** the server SHALL accept and persist the event with `event_type = "workflow_step"` in `span_events`

#### Scenario: Synthetic timeline types are rejected at ingest
- **WHEN** a client sends an event with `event_type: "span_started"`
- **THEN** the system SHALL reject the event with a validation error
- **AND** the same rejection SHALL apply to `"span_completed"` and `"span_failed"`

#### Scenario: Empty event type string is rejected
- **WHEN** a client sends an event with `event_type: ""`
- **THEN** the system SHALL reject the event with a validation error

---

### Requirement: Unknown Event Type Timeline Downgrade

When the timeline read path encounters a stored event whose `event_type` is not a recognized `TimelineEventType`, the system SHALL downgrade it to `custom` and preserve the original type in payload metadata.

_Note: The `default → custom` fallback already exists in `mapExplicitTimelineEventType`. The new behavior added here is the `__continua_original_event_type` metadata injection, which preserves the original type for consumers that need to distinguish downgraded events from intentional `custom` events._

The reserved metadata key SHALL be `payload.__continua_original_event_type`.

If the event has no existing payload, the system SHALL create a new payload object containing only the metadata key.

If the event has an existing payload, the system SHALL clone the payload once and append the metadata key without mutating the parsed original map.

#### Scenario: Unknown stored type downgrades to custom on timeline
- **WHEN** a stored event has `event_type = "workflow_step"` (not a recognized timeline type)
- **THEN** the timeline API SHALL return it with `event_type: "custom"`
- **AND** `payload.__continua_original_event_type` SHALL be `"workflow_step"`

#### Scenario: Downgrade with existing payload
- **WHEN** a stored event has `event_type = "workflow_step"` and an existing payload `{"key": "value"}`
- **THEN** the timeline API SHALL return `event_type: "custom"` with payload `{"key": "value", "__continua_original_event_type": "workflow_step"}`
- **AND** the original parsed payload map SHALL NOT be mutated

#### Scenario: Downgrade with absent payload
- **WHEN** a stored event has `event_type = "workflow_step"` and no payload
- **THEN** the timeline API SHALL return `event_type: "custom"` with payload `{"__continua_original_event_type": "workflow_step"}`

#### Scenario: Genuine custom events are not tagged as downgraded
- **WHEN** a stored event has `event_type = "custom"` (a recognized timeline type)
- **THEN** the timeline API SHALL return it with `event_type: "custom"`
- **AND** the payload SHALL NOT contain a `__continua_original_event_type` key

#### Scenario: Pagination remains correct with mixed event types
- **WHEN** a timeline contains `effect`, `wait`, known, and unknown-downgraded events
- **THEN** cursor pagination and poll-cursor traversal SHALL remain duplicate-free
