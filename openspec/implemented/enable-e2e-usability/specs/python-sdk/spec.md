## ADDED Requirements

### Requirement: Python SDK Client

The SDK SHALL provide a `Continua` client class for configuring and sending trace data to the Continua platform.

#### Scenario: Client initialization
- **WHEN** `Continua(api_key="...", endpoint="...")` is called
- **THEN** client is configured with API key and endpoint
- **AND** client is registered as singleton instance

#### Scenario: Synchronous ingest
- **WHEN** `client.ingest(batch)` is called
- **THEN** batch is sent via HTTP POST to `/v1/ingest`
- **AND** response is returned to caller

#### Scenario: Graceful shutdown
- **WHEN** `client.shutdown()` is called
- **THEN** pending batch data is flushed
- **AND** HTTP client is closed

### Requirement: Trace Decorator

The SDK SHALL provide a `@trace` decorator for instrumenting functions with trace context.

#### Scenario: Function traced
- **WHEN** decorated function is called
- **THEN** trace is created with function name
- **AND** trace is queued on function exit

#### Scenario: Custom trace name
- **WHEN** `@trace(name="custom_name")` is applied
- **THEN** trace uses specified name instead of function name

#### Scenario: Exception captured
- **WHEN** decorated function raises exception
- **THEN** trace status is set to "failed"
- **AND** exception is re-raised

### Requirement: Span Context Manager

The SDK SHALL provide a `span()` context manager for creating child spans within traces.

#### Scenario: Span within trace
- **WHEN** `with span(name="...", type="llm")` is used inside trace
- **THEN** span is created with parent trace ID
- **AND** span is queued on context exit

#### Scenario: Nested spans
- **WHEN** spans are nested
- **THEN** child span references parent span ID

#### Scenario: Span metadata
- **WHEN** `span.set_model()`, `span.set_tokens()` are called
- **THEN** metadata is included in span data

### Requirement: Automatic Batching

The SDK SHALL batch trace and span data for efficient transmission.

#### Scenario: Batch accumulation
- **WHEN** traces and spans are created
- **THEN** data accumulates in batch queue

#### Scenario: Automatic flush
- **WHEN** batch size threshold is reached OR flush interval elapses
- **THEN** accumulated data is sent to server

#### Scenario: Manual flush
- **WHEN** `client.flush()` is called
- **THEN** pending data is immediately sent
