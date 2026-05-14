## ADDED Requirements

### Requirement: Operator project selection
The system SHALL let signed-in operators choose an active project for debugger views.

#### Scenario: Operator lists projects
- **WHEN** a signed-in operator requests `GET /api/projects`
- **THEN** the response returns the available projects without exposing API key material

#### Scenario: Active project scopes list views
- **WHEN** a debugger list request is authenticated by Auth0 and includes `project_id`
- **THEN** only resources from that selected project are returned

### Requirement: Project selection compatibility with API keys
The system SHALL continue to derive project scope from API keys when a request is authenticated with a project API key.

#### Scenario: API key ignores selected project
- **WHEN** a debugger request is authenticated with a valid API key and also includes `project_id`
- **THEN** the API key's project scope is used
- **AND** the request is not re-scoped by the query parameter

### Requirement: Detail routes preserve selected project consistency
The system SHALL reject debugger detail access when a selected project is present and does not match the resource's owning project.

#### Scenario: Trace detail mismatched selected project
- **WHEN** a trace detail request includes a `project_id` query value for a different project than the trace
- **THEN** the response status is `404`

#### Scenario: Session detail mismatched selected project
- **WHEN** a session detail request includes a `project_id` query value for a different project than the session
- **THEN** the response status is `404`
