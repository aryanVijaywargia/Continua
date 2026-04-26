## MODIFIED Requirements

### Requirement: Ingest Event Type Enum Extension
The `IngestEventType` OpenAPI enum SHALL include `snapshot_marker` in addition to the existing values (`log`, `error`, `exception`, `message`, `metric`, `custom`, `state_change`, `decision`, `effect`, `wait`). The `TimelineEventType` enum SHALL include `snapshot_marker` alongside existing explicit and synthetic types.

#### Scenario: Ingest a snapshot_marker event
- **WHEN** a client sends `POST /v1/ingest` with an event having `event_type: "snapshot_marker"`
- **THEN** the event is accepted and stored
- **AND** the event appears in timeline responses with `event_type: "snapshot_marker"`

## ADDED Requirements

### Requirement: Snapshot Marker Backend Passthrough
The backend mapper SHALL recognize `snapshot_marker` as a valid explicit event type and map it through to the timeline response without degradation. The existing permissive ingest model SHALL apply: any event with `event_type: "snapshot_marker"` is accepted regardless of payload content.

#### Scenario: Mapper recognizes snapshot_marker
- **WHEN** a stored event has `event_type` value `snapshot_marker`
- **THEN** `mapExplicitTimelineEventType` returns `snapshot_marker` as a recognized type

#### Scenario: Malformed snapshot_marker accepted and retrievable
- **WHEN** a `snapshot_marker` event is ingested with an empty payload
- **THEN** the event is accepted and stored
- **AND** the event is retrievable via the timeline API with `event_type: "snapshot_marker"`
