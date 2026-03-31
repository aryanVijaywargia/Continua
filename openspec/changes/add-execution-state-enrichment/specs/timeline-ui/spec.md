## MODIFIED Requirements

### Requirement: Frontend Event Type Updates
The frontend timeline event type definitions SHALL include `"snapshot_marker"` as a valid event type in the manual TypeScript union in `web/src/api/client.ts`. Timeline event summary logic SHALL return meaningful summaries for `snapshot_marker` events when well-formed.

#### Scenario: Summarize a well-formed snapshot_marker
- **WHEN** a `snapshot_marker` event has payload `{"marker_kind": "milestone", "label": "Data ingestion complete"}`
- **THEN** `summarizeTimelineEvent` returns the `label` value as the summary text

#### Scenario: Summarize a malformed snapshot_marker
- **WHEN** a `snapshot_marker` event is missing `label` or `marker_kind` in its payload
- **AND** the event has a `message` field
- **THEN** `summarizeTimelineEvent` returns the `message` value

## ADDED Requirements

### Requirement: Snapshot Marker Payload Extraction
The frontend SHALL provide a `getSnapshotMarkerDetails()` function in `web/src/utils/eventSemantics.ts` that extracts `marker_kind` and `label` from a `snapshot_marker` event payload. The function SHALL return `null` when the event is not `snapshot_marker` type, or when `marker_kind` or `label` are missing or empty strings.

#### Scenario: Extract well-formed marker details
- **WHEN** an event has `event_type: "snapshot_marker"` and payload `{"marker_kind": "milestone", "label": "Pipeline ready"}`
- **THEN** `getSnapshotMarkerDetails` returns `{ markerKind: "milestone", label: "Pipeline ready" }`

#### Scenario: Extract from malformed marker
- **WHEN** an event has `event_type: "snapshot_marker"` but payload is missing `label`
- **THEN** `getSnapshotMarkerDetails` returns `null`

#### Scenario: Extract from wrong event type
- **WHEN** an event has `event_type: "custom"` with payload containing `marker_kind` and `label`
- **THEN** `getSnapshotMarkerDetails` returns `null`

### Requirement: Snapshot Marker Timeline Row Rendering
The `Timeline` component SHALL render `snapshot_marker` events with a dedicated collapsed-row preview when the marker is well-formed. The preview SHALL show `label` as primary text and `marker_kind` as a secondary pill. When the marker is malformed (missing `marker_kind` or `label`), the component SHALL fall back to generic timeline row rendering. Payload inspection and span navigation SHALL remain unchanged from existing event row behavior.

#### Scenario: Render well-formed snapshot_marker
- **WHEN** a timeline contains a `snapshot_marker` event with `marker_kind: "milestone"` and `label: "Data loaded"`
- **THEN** the Timeline renders a row with "Data loaded" as primary text and a "milestone" pill

#### Scenario: Render malformed snapshot_marker
- **WHEN** a timeline contains a `snapshot_marker` event without `label` in the payload
- **THEN** the Timeline renders the event as a generic event row

### Requirement: Snapshot Marker Semantic Filter Inclusion
The `snapshot_marker` event type SHALL be included in the existing `Semantic` filter mode alongside `state_change`, `decision`, `effect`, and `wait`. No new filter mode SHALL be added for milestone events.

#### Scenario: Semantic filter includes snapshot_marker
- **WHEN** a user selects the `Semantic` filter mode in the timeline
- **THEN** `snapshot_marker` events are visible alongside other semantic event types
- **AND** non-semantic event types (log, error, custom, etc.) are filtered out

#### Scenario: All mode includes snapshot_marker
- **WHEN** a user views the timeline in `All` mode
- **THEN** `snapshot_marker` events are visible among all other events
