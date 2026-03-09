## ADDED Requirements

### Requirement: Python SDK Async Mode Configuration

The Python SDK SHALL support explicit ingest mode configuration to control whether batches are submitted synchronously, via true async, or using the server default.

#### Scenario: Sync mode
- **WHEN** the SDK is configured with `ingest_mode="sync"`
- **THEN** all ingest requests include `sync=true` query parameter

#### Scenario: Async v2 mode
- **WHEN** the SDK is configured with `ingest_mode="async_v2"`
- **THEN** all ingest requests include the `X-Continua-Async-Version: 2` header

#### Scenario: Server default mode
- **WHEN** the SDK is configured with `ingest_mode="server_default"` or no mode is specified
- **THEN** ingest requests are sent without sync parameter or async header, deferring to server-side behavior

### Requirement: Batch Status Polling Helper

The Python SDK SHALL provide a `wait_for_batch()` helper that polls the batch status endpoint until the batch reaches a terminal state or a timeout expires.

#### Scenario: Wait for completed batch
- **WHEN** `wait_for_batch(batch_id, timeout=30)` is called and the batch completes within the timeout
- **THEN** the helper returns the final batch status response with `status: completed`

#### Scenario: Wait for failed batch
- **WHEN** `wait_for_batch(batch_id, timeout=30)` is called and the batch fails
- **THEN** the helper returns the final batch status response with `status: failed` including `last_error_code`

#### Scenario: Wait timeout exceeded
- **WHEN** `wait_for_batch(batch_id, timeout=5)` is called and the batch does not reach a terminal state within 5 seconds
- **THEN** the helper raises a timeout exception

#### Scenario: Configurable poll interval
- **WHEN** `wait_for_batch(batch_id, timeout=30, poll_interval=0.5)` is called
- **THEN** the helper polls every 0.5 seconds instead of the default interval

### Requirement: Flush Behavior Preservation

The Python SDK SHALL preserve existing fire-and-forget `flush()` behavior by default, regardless of async mode configuration.

#### Scenario: Flush remains fire-and-forget
- **WHEN** `flush()` is called in any ingest mode
- **THEN** it submits pending batches without waiting for processing completion

### Requirement: Migration Documentation

The Python SDK SHALL document the behavioral change from fake-async to true-async and provide migration guidance.

#### Scenario: Read-after-write warning
- **WHEN** a developer reads the SDK documentation for async mode
- **THEN** the documentation clearly states that true async is not read-after-write and provides alternatives (`sync=true` or `wait_for_batch()`)
