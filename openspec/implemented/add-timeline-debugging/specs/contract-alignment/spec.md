## ADDED Requirements

### Requirement: Timeline Contract Alignment

The OpenAPI contract, backend implementation, Python SDK types, and web client types SHALL be aligned for all timeline-related types and endpoints.

#### Scenario: OpenAPI compiles cleanly
- **WHEN** `make generate` is run after all Phase 4 OpenAPI changes
- **THEN** the generated server code compiles without errors

#### Scenario: Backend responses match contract
- **WHEN** the timeline endpoint returns a response
- **THEN** the response shape matches the `TimelineResponse` schema in OpenAPI exactly

#### Scenario: Event type enums correctly separated
- **WHEN** event types are used across OpenAPI, backend, and SDK
- **THEN** the ingest contract (`IngestEventType`) accepts only explicit types (`log`, `error`, `exception`, `message`, `metric`, `custom`)
- **AND** the timeline response (`TimelineEventType`) includes both explicit and synthetic types (`span_started`, `span_completed`, `span_failed`)

#### Scenario: Web client types match timeline contract
- **WHEN** the manual types in `web/src/api/client.ts` are reviewed
- **THEN** they include correct `TimelineEvent` and `TimelineResponse` types matching the OpenAPI schema

#### Scenario: Python SDK types current
- **WHEN** `make generate` is run
- **THEN** any generated Python types reflect the current OpenAPI contract
