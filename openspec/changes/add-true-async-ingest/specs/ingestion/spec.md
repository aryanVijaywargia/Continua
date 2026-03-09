## ADDED Requirements

### Requirement: Batch Ingestion Endpoint

The system SHALL provide a `POST /v1/ingest` endpoint that accepts batched trace data and supports both synchronous processing and staged asynchronous acceptance.

#### Scenario: Successful sync ingestion
- **Given** a valid ingest request with `batch_key`, traces, spans, and events
- **And** query param `sync=true`
- **WHEN** the request is sent to `POST /v1/ingest?sync=true`
- **THEN** the response status is `200`
- **AND** the response body contains `status: ok`, `batch_key`, and `batch_id`
- **AND** all traces, spans, and events are persisted before the response returns

#### Scenario: Legacy async behavior remains during Stage A default
- **Given** a valid ingest request
- **And** no async-version header is present
- **And** `INGEST_TRUE_ASYNC_DEFAULT=false`
- **WHEN** the request is sent to `POST /v1/ingest`
- **THEN** the response status is `202`
- **AND** the response body contains `status: accepted` and `batch_key`
- **AND** the request is processed inline for backward compatibility

#### Scenario: True async acceptance returns batch identity
- **Given** a valid ingest request
- **And** either `X-Continua-Async-Version: 2` is present or server default true async is enabled
- **WHEN** the request is sent to `POST /v1/ingest`
- **THEN** the response status is `202`
- **AND** the response body contains `status: accepted`, `batch_key`, and `batch_id`
- **AND** the request is durably accepted without writing domain rows inline

#### Scenario: Unsupported async version is rejected
- **WHEN** a client sends `POST /v1/ingest` with `X-Continua-Async-Version` set to an unsupported value
- **THEN** the response status is `400`
- **AND** no batch is created

#### Scenario: Request too large
- **Given** an ingest request larger than 5MB
- **WHEN** the request is sent to `POST /v1/ingest`
- **THEN** the response status is `413`
- **AND** the response body contains `{"error": "batch exceeds 5MB limit"}`

### Requirement: Span Upsert with Trace UUID FK

The system SHALL upsert spans using the internal trace UUID as foreign key while preserving sync validation behavior and true-async dependency handling.

#### Scenario: Create span for existing trace
- **Given** a trace with `trace_id: "trace-001"` exists with internal UUID `abc-123`
- **WHEN** an ingest request contains a span with `trace_id: "trace-001"`, `span_id: "span-001"`
- **THEN** the span is created with `spans.trace_id = abc-123`

#### Scenario: Sync unknown trace reference fails immediately
- **Given** no trace with `trace_id: "trace-999"` exists
- **And** the request uses `sync=true`
- **WHEN** an ingest request contains only a span referencing that trace
- **THEN** the request fails with status `400`
- **AND** the error message indicates an unknown trace reference

#### Scenario: True async unknown trace reference retries before failing
- **Given** a true-async batch contains a span referencing a trace not yet visible in the database
- **WHEN** the worker first processes the batch
- **THEN** the batch is re-queued instead of failing immediately
- **AND** it is retried until the dependency retry window expires or the trace becomes visible
