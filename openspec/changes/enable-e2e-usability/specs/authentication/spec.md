## ADDED Requirements

### Requirement: API Key Authentication Middleware

The system SHALL enforce API key authentication on all data endpoints using middleware that validates keys and injects project context.

#### Scenario: Missing API key rejected
- **WHEN** request to protected endpoint lacks API key
- **THEN** response status is 401
- **AND** response body is `{"error":"missing API key"}`

#### Scenario: Invalid API key rejected
- **WHEN** request contains invalid API key in `X-API-Key` header
- **THEN** response status is 401
- **AND** response body is `{"error":"invalid API key"}`

#### Scenario: Valid API key accepted
- **WHEN** request contains valid API key in `X-API-Key` header
- **THEN** request is forwarded to handler
- **AND** project ID is available in request context

#### Scenario: Bearer token support
- **WHEN** request contains valid API key in `Authorization: Bearer <key>` header
- **THEN** request is authenticated successfully

### Requirement: Health Endpoint Public Access

The system SHALL allow unauthenticated access to the health endpoint via router composition (health routed outside OpenAPI middleware group).

#### Scenario: Health check without auth
- **WHEN** request to `GET /api/health` lacks API key
- **THEN** response status is 200
- **AND** health information is returned

#### Scenario: Health not in OpenAPI spec
- **WHEN** OpenAPI spec is examined
- **THEN** `/api/health` path is NOT defined
- **AND** health handler is registered directly in router code

### Requirement: Project Context Injection

The system SHALL inject the authenticated project ID into request context for use by handlers.

#### Scenario: Handler accesses project ID
- **WHEN** authenticated request reaches handler
- **THEN** handler can retrieve project ID from context
- **AND** handler uses project ID to scope database queries

### Requirement: Multi-Tenancy Data Isolation

The system SHALL ensure each project can only access its own data across ALL data endpoints.

#### Scenario: ListTraces scoped by project
- **WHEN** project A requests `GET /api/traces`
- **THEN** only traces belonging to project A are returned
- **AND** traces from project B are not visible

#### Scenario: GetTrace scoped by project
- **WHEN** project A requests `GET /api/traces/{id}` for a trace owned by project B
- **THEN** response status is 404 (not 403, to avoid information leakage)

#### Scenario: ListSpansByTrace scoped by project
- **WHEN** project A requests `GET /api/traces/{id}/spans` for a trace owned by project B
- **THEN** response status is 404

#### Scenario: ListSessions scoped by project
- **WHEN** project A requests `GET /api/sessions`
- **THEN** only sessions belonging to project A are returned

#### Scenario: Ingest scoped by project
- **WHEN** project A ingests data via `POST /v1/ingest`
- **THEN** all traces, spans, and events are associated with project A
