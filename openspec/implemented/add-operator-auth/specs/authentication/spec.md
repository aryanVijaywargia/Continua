## ADDED Requirements

### Requirement: Operator bearer authentication
The system SHALL authenticate debugger operator requests with Auth0 bearer tokens validated against the configured issuer and audience.

#### Scenario: Valid operator token accepted
- **WHEN** a debugger request includes a valid Auth0 bearer token
- **THEN** the request is authenticated
- **AND** operator identity is made available to the backend

#### Scenario: Invalid bearer token rejected
- **WHEN** a debugger request includes an invalid or expired bearer token
- **THEN** the response status is `401`
- **AND** the response uses the standard API error schema

### Requirement: Operator email allowlist
The system SHALL only allow debugger access for Auth0-authenticated operators whose email is present in the configured allowlist.

#### Scenario: Allowlisted operator allowed
- **WHEN** the validated Auth0 token resolves to an allowlisted email address
- **THEN** the debugger request is authorized

#### Scenario: Non-allowlisted operator rejected
- **WHEN** the validated Auth0 token resolves to an email address that is not allowlisted
- **THEN** the response status is `403`
- **AND** the response uses the standard API error schema

### Requirement: Split auth modes by surface
The system SHALL keep ingest authentication API-key based while allowing debugger and engine API routes to accept either API keys or Auth0 bearer tokens.

#### Scenario: Ingest remains API-key only
- **WHEN** a request targets ingest endpoints without a valid API key
- **THEN** the response status is `401`

#### Scenario: Debugger API key fallback remains valid
- **WHEN** a debugger read request includes a valid project API key
- **THEN** the request is authenticated without Auth0

### Requirement: Runtime auth config endpoint
The system SHALL expose a public runtime auth config endpoint for the embedded SPA.

#### Scenario: Runtime config available
- **WHEN** the web app requests `GET /api/auth/config`
- **THEN** the response status is `200`
- **AND** the payload includes the Auth0 settings required to bootstrap the SPA
