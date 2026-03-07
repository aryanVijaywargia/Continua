## ADDED Requirements

### Requirement: Events Timeline API Endpoint

The API SHALL expose a `GET /api/traces/{id}/events` endpoint that returns a merged chronological timeline of explicit events and synthetic span lifecycle markers for a trace.

#### Scenario: Timeline returns explicit events
- **WHEN** a trace has span events in `span_events`
- **THEN** the timeline response includes those events with `source: "explicit"` and their persisted `event_type`, `level`, `message`, and `payload`

#### Scenario: Timeline returns synthetic lifecycle markers
- **WHEN** a trace has spans with start/end times
- **THEN** the timeline response includes synthetic `span_started`, `span_completed`, or `span_failed` events with `source: "synthetic"` derived from span lifecycle data

#### Scenario: Synthetic span_started generation
- **WHEN** a span has a non-null `start_time`
- **THEN** a `span_started` synthetic event is generated with that timestamp

#### Scenario: Synthetic span_completed generation
- **WHEN** a span has a non-null `end_time` AND status is `completed`
- **THEN** a `span_completed` synthetic event is generated with that timestamp

#### Scenario: Synthetic span_failed generation
- **WHEN** a span has a non-null `end_time` AND status is `failed` or `error`
- **THEN** a `span_failed` synthetic event is generated with that timestamp

#### Scenario: Timeline is chronologically ordered
- **WHEN** the timeline is fetched
- **THEN** all events (explicit and synthetic) are returned in ascending order by display timestamp, with ties broken by: source (explicit before synthetic), sequence/lifecycle phase, then ID

#### Scenario: Orphan explicit events included
- **WHEN** an explicit event references a `span_id` that has no matching span row
- **THEN** the event still appears in the timeline with `span_name` absent

#### Scenario: Trace not found
- **WHEN** the trace ID does not exist or belongs to a different project
- **THEN** the API returns 404

### Requirement: Timeline Response Shape

The timeline response SHALL include `events`, `trace_status`, `has_more`, an optional `next_cursor`, and an optional `poll_cursor`.

#### Scenario: Response includes trace status
- **WHEN** the timeline is fetched
- **THEN** `trace_status` reflects the current trace status (`RUNNING`, `COMPLETED`, or `FAILED`)

#### Scenario: Response includes pagination metadata
- **WHEN** more events exist beyond the requested limit
- **THEN** `has_more` is `true` and `next_cursor` contains an opaque cursor string

#### Scenario: No more events
- **WHEN** all events have been returned
- **THEN** `has_more` is `false` and `next_cursor` is absent

#### Scenario: Response includes poll cursor
- **WHEN** the timeline response contains events
- **THEN** `poll_cursor` contains an opaque cursor for the last event included in the response
- **AND** the client can use `poll_cursor` for incremental polling even when `has_more` is `false`

### Requirement: Timeline Event Shape

Each timeline event SHALL include `id`, `trace_id`, `event_type`, `timestamp`, `source`, and optional `span_id`, `span_name`, `level`, `sequence`, `message`, and `payload`.

#### Scenario: Explicit event fields
- **WHEN** an explicit event is in the timeline
- **THEN** it includes the persisted `id`, `span_id`, `event_type`, `level`, `sequence`, `message`, `payload`, and `source: "explicit"`

#### Scenario: Synthetic event fields
- **WHEN** a synthetic event is in the timeline
- **THEN** it includes a deterministic `id` derived from span_id and event type, the `span_id`, `span_name`, `event_type`, and `source: "synthetic"`

### Requirement: Opaque Cursor Pagination

The timeline endpoint SHALL support opaque cursor-based pagination via an `after` query parameter.

#### Scenario: First page without cursor
- **WHEN** no `after` parameter is provided
- **THEN** the first page of events is returned from the beginning of the timeline

#### Scenario: Subsequent page with cursor
- **WHEN** `after` is set to a `next_cursor` from a previous response
- **THEN** events after that cursor position are returned with no duplicates

#### Scenario: Stable cursor ordering
- **WHEN** the same cursor is used in repeated requests (no data changes)
- **THEN** the same events are returned in the same order

#### Scenario: Running trace cursor consistency
- **WHEN** a running trace is polled incrementally via cursor
- **THEN** no duplicate events are returned, but late-arriving events with earlier display timestamps may be missed until a full refresh

#### Scenario: Terminal trace full refresh
- **WHEN** a trace becomes terminal (`COMPLETED` or `FAILED`)
- **THEN** the client performs a cursorless full fetch to get the complete, accurately-ordered timeline

#### Scenario: Invalid or malformed cursor
- **WHEN** the `after` parameter contains an invalid or malformed cursor string
- **THEN** the API returns 400 with error code `invalid_cursor`

#### Scenario: Limit parameter
- **WHEN** a `limit` query parameter is provided
- **THEN** at most `limit` events are returned (default: 100, max: 500)

### Requirement: Timeline Event Types

The timeline response SHALL use a `TimelineEventType` enum that includes both explicit types (`log`, `error`, `exception`, `message`, `metric`, `custom`) and synthetic lifecycle types (`span_started`, `span_completed`, `span_failed`). This enum is distinct from the `IngestEventType` enum used by the ingest contract, which only accepts explicit types.

#### Scenario: Explicit event types preserved
- **WHEN** an explicit event with type `log`, `error`, `exception`, `message`, `metric`, or `custom` is ingested
- **THEN** it appears in the timeline with its original `event_type`

#### Scenario: Synthetic event types generated
- **WHEN** span lifecycle data is processed
- **THEN** `span_started`, `span_completed`, or `span_failed` synthetic events are generated as appropriate

#### Scenario: Ingest rejects synthetic types
- **WHEN** an SDK attempts to ingest an event with `event_type` set to `span_started`, `span_completed`, or `span_failed`
- **THEN** the ingest contract rejects it as an invalid event type

### Requirement: Project Scoping

The timeline endpoint SHALL enforce project-level isolation.

#### Scenario: Timeline scoped to project
- **WHEN** the authenticated project requests a trace timeline
- **THEN** only traces belonging to that project are accessible
- **AND** traces from other projects return 404
