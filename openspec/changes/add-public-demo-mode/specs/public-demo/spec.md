## ADDED Requirements

### Requirement: Public read-only demo mode
The system SHALL provide an env-controlled public demo mode for the debugger console.

#### Scenario: Demo console loads without sign-in
- **WHEN** `PUBLIC_DEMO_ENABLED` is set for a deployment
- **THEN** `/dashboard`, `/traces`, `/traces/{id}`, `/sessions`, `/sessions/{id}`, and `/sessions/{id}/compare` load without Auth0 sign-in

### Requirement: Demo reads are server-scoped
The system SHALL force all public demo reads to a single configured project on the server.

#### Scenario: Query parameter cannot escape demo scope
- **WHEN** a public demo request includes a `project_id` query parameter
- **THEN** the server ignores the caller-provided value
- **AND** returns only resources from `PUBLIC_DEMO_PROJECT_ID`

#### Scenario: Cross-project detail access is rejected
- **WHEN** a public demo request targets a trace or session outside `PUBLIC_DEMO_PROJECT_ID`
- **THEN** the response status is `404`

### Requirement: Demo mode stays read-only
The system SHALL keep ingest and control routes non-public in demo mode.

#### Scenario: Ingest remains protected
- **WHEN** a caller targets `/v1/ingest` without a valid API key during demo mode
- **THEN** the response status is `401`

### Requirement: Demo UX communicates the environment
The system SHALL identify public demo mode clearly in the debugger shell and landing page.

#### Scenario: Demo shell hides operator-only chrome
- **WHEN** the debugger renders in public demo mode
- **THEN** the project switcher, account/session controls, and settings entry are hidden or redirected
- **AND** the shell presents read-only sample-data messaging with a run-locally CTA

### Requirement: Local self-host console remains usable
The system SHALL keep a local API-key console path for non-demo self-hosted usage.

#### Scenario: Non-demo local console accepts project API key
- **WHEN** public demo mode is disabled
- **AND** Auth0 runtime config is not enabled
- **THEN** protected debugger routes prompt for a project API key
- **AND** successful entry stores the key locally for local API requests

#### Scenario: Public demo never uses a browser project key
- **WHEN** public demo mode is enabled
- **THEN** debugger reads are unauthenticated from the browser
- **AND** the server provides project scope from `PUBLIC_DEMO_PROJECT_ID`
