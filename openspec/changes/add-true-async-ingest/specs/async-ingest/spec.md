## ADDED Requirements

### Requirement: True Async Batch Acceptance

The system SHALL accept ingest batches durably and queue them for background processing when true async is active. Acceptance MUST atomically commit the batch record in `queued`, the compressed payload, and the River job in one database transaction. The system SHALL return `202` only after that transaction commits.

#### Scenario: Async acceptance with opt-in header
- **WHEN** a client sends `POST /v1/ingest` with `X-Continua-Async-Version: 2` and a valid batch
- **THEN** the system returns `202` with `status: accepted`, `batch_key`, and `batch_id`
- **AND** no trace, span, or event rows are written at acceptance time

#### Scenario: Duplicate batch returns existing batch_id
- **WHEN** a client submits a batch with a `batch_key` that already exists for the project
- **THEN** the system returns `202` with `status: duplicate` and the existing `batch_id`

#### Scenario: Acceptance-time validation rejects client-visible mistakes
- **WHEN** a client submits invalid JSON, a missing `batch_key`, invalid enum/value fields, or other structurally invalid fields
- **THEN** the system returns `400` immediately without creating a batch record

#### Scenario: Unsupported async version is rejected
- **WHEN** a client sends `X-Continua-Async-Version` with a value other than `2`
- **THEN** the system returns `400`
- **AND** no batch record is created

#### Scenario: Body size enforcement
- **WHEN** a client submits a request body exceeding 5MB
- **THEN** the system returns `413` without creating a batch record

### Requirement: Async Worker Processing

The system SHALL process queued batches via a River worker with explicit state-claim and data-processing boundaries.

#### Scenario: Processing state becomes externally visible
- **WHEN** the worker picks up a `queued` batch
- **THEN** it commits a transition to `processing` with `processing_started_at` and incremented `attempt_count` before heavy work begins

#### Scenario: Successful batch processing
- **WHEN** a worker has already committed `processing` for a batch
- **THEN** it processes all traces, spans, and events
- **AND** enqueues rollup jobs
- **AND** marks the batch `completed`
- **AND** deletes the payload in the same data transaction

#### Scenario: Retryable failure
- **WHEN** the worker encounters a transient error such as a database error or transaction conflict
- **THEN** it rolls back the data transaction
- **AND** records the error in `last_error_*`
- **AND** moves the batch back to `queued`
- **AND** returns an error so River retries

#### Scenario: Dependency not ready retries
- **WHEN** the worker encounters an unknown trace reference that may be satisfied by another accepted batch still in flight
- **THEN** it records `last_error_code='dependency_not_ready'`
- **AND** moves the batch back to `queued`
- **AND** retries according to River backoff

#### Scenario: Dependency wait expires
- **WHEN** a batch still has unresolved trace references after the configured dependency retry window
- **THEN** it is marked `failed` with `last_error_code='reference_timeout'`
- **AND** no partial writes are committed

#### Scenario: Terminal processing failure
- **WHEN** the worker encounters a terminal processing error such as `payload_decode_error`
- **THEN** it rolls back the data transaction
- **AND** marks the batch `failed` with `last_error_code` and `last_error_message`
- **AND** does not retry

#### Scenario: Idempotent re-entry after completion
- **WHEN** the worker retries a batch that is already `completed`
- **THEN** it returns success without reprocessing

#### Scenario: Idempotent re-entry for failed batch
- **WHEN** the worker retries a batch that is already `failed`
- **THEN** it returns success without reprocessing

#### Scenario: Error preservation during retries
- **WHEN** the worker retries a batch
- **THEN** it preserves `last_error_code`, `last_error_message`, and `last_error_at` from the previous attempt until success clears them

### Requirement: Staged Rollout

The system SHALL support a staged rollout for true async ingest while preserving backward compatibility during Stage A.

#### Scenario: Stage A legacy default without header
- **WHEN** a client sends `POST /v1/ingest` without `X-Continua-Async-Version: 2` and `INGEST_TRUE_ASYNC_DEFAULT=false`
- **THEN** the system uses legacy fake-async behavior and returns `202`

#### Scenario: Stage A opt-in with header
- **WHEN** a client sends `POST /v1/ingest` with `X-Continua-Async-Version: 2`
- **THEN** the system uses true async acceptance and background processing

#### Scenario: Stage C env-controlled default
- **WHEN** `INGEST_TRUE_ASYNC_DEFAULT=true` and no header is present
- **THEN** non-sync requests use true async by default

#### Scenario: sync=true always inline
- **WHEN** `sync=true` is set regardless of header or env config
- **THEN** the system always processes inline and returns `200`

### Requirement: Payload Storage and Retention

The system SHALL store raw request payloads separately from batch metadata and manage payload lifecycle independently.

#### Scenario: Payload stored at acceptance
- **WHEN** a batch is accepted via true-async path
- **THEN** the compressed payload is stored in `ingest_batch_payloads` with `batch_id`, `byte_size`, and `compression` metadata

#### Scenario: Payload deleted on success
- **WHEN** a batch completes processing successfully
- **THEN** the payload row is deleted in the same transaction that marks the batch `completed`

#### Scenario: Failed payload retained
- **WHEN** a batch is marked `failed`
- **THEN** the payload row is retained for 7 days for debugging and later pruned by the cleanup job

#### Scenario: Batch metadata retained for idempotency
- **WHEN** a batch reaches `completed` or `failed`
- **THEN** its `ingest_batches` row is retained as the durable idempotency record

### Requirement: Sync Path Enhancement

The system SHALL add `batch_id` to sync responses without changing existing sync processing semantics.

#### Scenario: Sync response includes batch_id
- **WHEN** a client sends `POST /v1/ingest` with `sync=true` and a new batch
- **THEN** the response includes `batch_id` alongside `status: ok`, `batch_key`, and counts

#### Scenario: Sync duplicate includes batch_id
- **WHEN** a client sends a sync duplicate batch
- **THEN** the response includes the existing `batch_id`

### Requirement: Database Migration

The system SHALL migrate `ingest_batches` to support true async using a compatibility-first rollout.

#### Scenario: Additive schema migration is backward compatible
- **WHEN** the first migration runs on a database used by old and new server versions
- **THEN** it only adds new columns and tables needed for true async
- **AND** it does not require legacy status values to be rewritten immediately

#### Scenario: Post-cutover backfill converts legacy statuses
- **WHEN** all servers have been upgraded to the new async ingest implementation
- **THEN** a follow-up backfill converts legacy `accepted` rows to `completed`
- **AND** stale legacy `processing` rows older than the grace window are converted to `failed` with `last_error_code='legacy_interrupted'`

### Requirement: Batch Cleanup

The system SHALL prune expired payload/debug artifacts without deleting idempotency records.

#### Scenario: Cleanup prunes old failed payloads
- **WHEN** the daily cleanup job runs
- **THEN** it deletes payload rows for `failed` batches older than 7 days

#### Scenario: Batch rows are retained
- **WHEN** the cleanup job runs
- **THEN** it does not delete terminal `ingest_batches` rows that enforce batch-key idempotency

#### Scenario: Active batches never have payloads pruned
- **WHEN** the cleanup job runs
- **THEN** payloads for batches in `queued` or `processing` are never deleted
