## ADDED Requirements

### Requirement: Sessions API Support

The API SHALL expose session detail and list fields required by the UI: id, name, user_id, created_at, and trace_count.

#### Scenario: Session detail endpoint
- **WHEN** GET /api/sessions/{id} is called
- **THEN** the session is returned with all fields or 404 if it does not exist

#### Scenario: Session list includes trace counts
- **WHEN** GET /api/sessions is called
- **THEN** each session includes id, name, user_id, trace_count, and created_at

#### Scenario: Session schema extended
- **WHEN** the Session OpenAPI schema is reviewed
- **THEN** it includes user_id (string, nullable) and trace_count (integer)

#### Scenario: trace_count computed per session
- **WHEN** a session is returned from the API
- **THEN** trace_count equals the count of traces with the same session_id and project_id

#### Scenario: Session with zero traces
- **WHEN** a session has no traces
- **THEN** it appears in the sessions list with trace_count=0

### Requirement: Sessions List Page

The web UI SHALL provide a paginated list of sessions.

#### Scenario: Sessions page loads
- **WHEN** a user navigates to /sessions
- **THEN** a table of sessions is displayed
- **AND** columns include: session_id, user_id, trace_count, created_at (name if present)

#### Scenario: Sessions pagination
- **WHEN** more sessions exist than page size
- **THEN** pagination controls are shown
- **AND** user can navigate between pages using Previous/Next

#### Scenario: Session row click
- **WHEN** a user clicks on a session row
- **THEN** they are navigated to /sessions/:id

### Requirement: Session Detail Page

The web UI SHALL provide a detail view for individual sessions.

#### Scenario: Session detail loads
- **WHEN** a user navigates to /sessions/:id
- **THEN** session metadata is displayed in header
- **AND** related traces are listed below

#### Scenario: Session traces list
- **WHEN** viewing session detail
- **THEN** only traces belonging to that session are shown
- **AND** traces are paginated

#### Scenario: Navigate to trace detail
- **WHEN** a user clicks on a trace in session detail
- **THEN** they are navigated to /traces/:id

#### Scenario: Navigate back to sessions
- **WHEN** a user is on session detail page
- **THEN** a back link to /sessions is available

### Requirement: Sessions Navigation

The web UI SHALL include sessions in main navigation.

#### Scenario: Sessions link in nav
- **WHEN** the user is on any page
- **THEN** a "Sessions" link is visible in navigation
- **AND** clicking it navigates to /sessions
