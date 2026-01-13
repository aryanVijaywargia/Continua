## ADDED Requirements

### Requirement: Server Bootstrap with Fx

The system SHALL start an HTTP server using Uber Fx for dependency injection, wiring together configuration, database pool, store, ingest service, and API handlers.

#### Scenario: Server starts successfully
- **WHEN** `continua serve` is executed with valid `DATABASE_URL`
- **THEN** the server starts listening on configured port
- **AND** all Fx modules are initialized in correct order

#### Scenario: Health endpoint accessible
- **WHEN** server is running
- **AND** client requests `GET /api/health`
- **THEN** response status is 200
- **AND** response body contains `{"status":"ok","version":"..."}`

#### Scenario: Graceful shutdown
- **WHEN** server receives SIGINT or SIGTERM
- **THEN** server stops accepting new connections
- **AND** waits for in-flight requests to complete
- **AND** closes database connections cleanly

### Requirement: Configuration Loading (Env-Only for Phase 2)

The system SHALL load configuration from environment variables with sensible defaults. YAML-based configuration (`config.example.yaml`) is deferred to future phases.

#### Scenario: Default configuration
- **WHEN** server starts without explicit environment variables
- **THEN** server binds to `0.0.0.0:8080`

#### Scenario: Custom port
- **WHEN** `PORT=9000` environment variable is set
- **THEN** server binds to port 9000

#### Scenario: Database URL required
- **WHEN** `DATABASE_URL` is not set
- **THEN** server fails to start with clear error message

#### Scenario: YAML config ignored in Phase 2
- **WHEN** `config.example.yaml` exists in repository
- **THEN** server does NOT read from it
- **AND** only environment variables are used for configuration

### Requirement: HTTP Router Composition

The system SHALL assemble Chi router using composition pattern: health endpoint routed directly (public), OpenAPI handlers mounted under auth middleware group.

#### Scenario: Health endpoint public
- **WHEN** router is assembled
- **THEN** `/api/health` is routed directly without auth middleware
- **AND** health endpoint is NOT defined in OpenAPI spec (removed)

#### Scenario: Protected routes under middleware
- **WHEN** router is assembled
- **THEN** all OpenAPI-defined endpoints receive auth middleware
- **AND** no path-based bypass logic is needed in middleware

#### Scenario: API routes mounted
- **WHEN** router is assembled
- **THEN** all OpenAPI endpoints are accessible at their defined paths
- **AND** web UI is served at root path `/`

#### Scenario: Middleware applied
- **WHEN** request is received
- **THEN** request ID is assigned
- **AND** request is logged
- **AND** panics are recovered

### Requirement: OpenAPI Schema Extensions

The system SHALL extend OpenAPI schemas to support UI requirements.

#### Scenario: Trace schema has error_count
- **WHEN** trace is returned from API
- **THEN** response includes `error_count` field (integer, nullable)

#### Scenario: Span schema has input/output
- **WHEN** span is returned from API
- **THEN** response includes `input` field (object, nullable)
- **AND** response includes `output` field (object, nullable)

#### Scenario: Span schema has parent_span_id as string (not UUID)
- **WHEN** span has a parent span
- **THEN** response includes `parent_span_id` field as string type (not UUID)
- **AND** value is the external span ID directly from DB (no UUID lookup)
- **AND** OpenAPI schema specifies `type: string` without `format: uuid`
