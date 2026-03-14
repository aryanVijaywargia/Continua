## MODIFIED Requirements

### Requirement: Trace Schema Extensions

The OpenAPI Trace schema SHALL include fields needed by the web UI.

#### Scenario: Error count exposed
- **WHEN** Trace schema is extended
- **THEN** `error_count` field is added (integer type)
- **AND** field represents count of failed spans in trace

#### Scenario: Error count in API response
- **WHEN** GetTrace or ListTraces is called
- **THEN** response includes `error_count` value
- **AND** mapper populates from database `error_count` column

### Requirement: Span Schema Extensions

The OpenAPI Span schema SHALL include fields needed by span tree visualization and detail panel.

#### Scenario: Parent span ID as string
- **WHEN** Span schema is extended
- **THEN** `parent_span_id` type is changed from UUID to string
- **AND** field stores external SDK identifier, not internal UUID

#### Scenario: Input payload exposed
- **WHEN** Span schema is extended
- **THEN** `input` field is added (object type, nullable)
- **AND** field contains input payload JSONB from database

#### Scenario: Output payload exposed
- **WHEN** Span schema is extended
- **THEN** `output` field is added (object type, nullable)
- **AND** field contains output payload JSONB from database

#### Scenario: Payloads in API response
- **WHEN** GetSpansByTrace is called
- **THEN** each span includes `input` and `output` if present
- **AND** mapper parses JSONB from database columns

### Requirement: Health Endpoint Routing

The `/api/health` endpoint SHALL be removed from OpenAPI and routed separately.

#### Scenario: Health removed from OpenAPI
- **WHEN** OpenAPI spec is updated
- **THEN** `/api/health` path is removed
- **AND** spec only contains protected data endpoints

#### Scenario: Health routed separately
- **WHEN** router is assembled
- **THEN** health endpoint is registered before OpenAPI routes
- **AND** health endpoint does not receive middleware

### Requirement: Type Regeneration

After OpenAPI changes, ALL generated types SHALL be regenerated.

#### Scenario: Go types regenerated
- **WHEN** `make generate` is run
- **THEN** `internal/api/server_gen.go` includes new fields
- **AND** Trace struct has `ErrorCount` field
- **AND** Span struct has `ParentSpanId`, `Input`, `Output` fields (as strings/objects)

#### Scenario: Python types regenerated
- **WHEN** `make generate` is run
- **THEN** `sdks/python/src/continua/types.py` includes new fields
- **AND** Pydantic models match updated schema

#### Scenario: TypeScript types regenerated
- **WHEN** `make generate` is run
- **THEN** `contracts/generated/typescript/api.ts` includes new fields
- **AND** TypeScript interfaces match updated schema

### Requirement: Mapper Updates

The API mappers SHALL be updated to populate new fields from database.

#### Scenario: Trace mapper includes error count
- **WHEN** `traceToAPI` is called
- **THEN** `trace.ErrorCount` is set from `t.ErrorCount` database column

#### Scenario: Span mapper includes parent span ID
- **WHEN** `spanToAPI` is called
- **THEN** `span.ParentSpanId` is set from `sp.ParentSpanID` TEXT column

#### Scenario: Span mapper includes payloads
- **WHEN** `spanToAPI` is called
- **AND** span has input JSONB
- **THEN** input is parsed and assigned to `span.Input`
- **AND** if span has output JSONB
- **THEN** output is parsed and assigned to `span.Output`
