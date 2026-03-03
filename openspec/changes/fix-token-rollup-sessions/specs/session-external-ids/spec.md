## ADDED Requirements

### Requirement: External Session Identifiers

The system SHALL support human-readable external session identifiers that are mapped to internal UUIDs server-side. SDK users SHALL pass string session keys (e.g., `"checkout-flow-test-42"`) and never deal with session UUIDs. All session_id strings — including UUID-looking ones — are treated as external keys.

#### Scenario: Lazy session creation on ingest

- **WHEN** a trace is ingested with a `session_id` string
- **THEN** the server calls `GetOrCreateSessionByExternalID` with the project ID and the string key
- **AND** a new session is created if none exists for that `(project_id, external_id)` pair
- **AND** the trace is linked to the resolved internal session UUID

#### Scenario: Existing session reuse by external ID

- **WHEN** a trace is ingested with a `session_id` that matches an existing session's `external_id` for the same project
- **THEN** the existing session's internal UUID is used as the trace's session FK
- **AND** the session's `updated_at` timestamp is refreshed

#### Scenario: Concurrent session creation safety

- **WHEN** two ingest requests simultaneously reference the same `(project_id, external_id)` pair
- **THEN** exactly one session is created and both traces reference the same session UUID
- **AND** no duplicate key errors are raised

### Requirement: Session External ID in API Responses

The system SHALL include the `external_id` field in session API responses so clients can correlate sessions with their human-readable identifiers.

#### Scenario: Session detail response includes external ID

- **WHEN** a client requests `GET /api/sessions/{id}`
- **THEN** the API response includes the `external_id` field with the original string key

#### Scenario: Session list response includes external ID

- **WHEN** a client requests `GET /api/sessions`
- **THEN** each returned session includes the `external_id` field
- **AND** `trace_count` remains accurate per `(project_id, session_id)`

### Requirement: Ingest Session Identifier Is an External String Key

The system SHALL define `IngestTraceInput.session_id` as a plain string external key in the API contract (not UUID-formatted).

#### Scenario: Non-UUID session keys are accepted

- **WHEN** a client ingests a trace with `session_id: "checkout-flow-42"`
- **THEN** request parsing succeeds without UUID validation
- **AND** the server resolves or creates the session by `external_id`
