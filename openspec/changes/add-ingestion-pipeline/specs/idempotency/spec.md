# Spec: Idempotency

## Overview

Batch idempotency capability ensuring retry-safe ingestion with deduplication.

---

## ADDED Requirements

### Requirement: Batch Claiming at Transaction Start

The system SHALL claim batch idempotency as the FIRST operation in any ingest transaction.

#### Scenario: Claim new batch
- **Given**: No batch with `batch_key: "batch-001"` exists for the project
- **When**: The ingest transaction begins
- **Then**: A new `ingest_batches` record is created with `status: "processing"`
- **And**: The batch ID is returned for subsequent operations

#### Scenario: Claim duplicate batch
- **Given**: A batch with `batch_key: "batch-001"` already exists (any status)
- **When**: The ingest transaction attempts to claim
- **Then**: The INSERT returns no rows (ON CONFLICT DO NOTHING)
- **And**: `ErrDuplicateBatch` error is returned
- **And**: The transaction is rolled back immediately

---

### Requirement: Duplicate Returns Success

The system SHALL return success (not error) for duplicate batch submissions.

#### Scenario: Sync duplicate returns 200
- **Given**: Batch `batch-001` was already processed successfully
- **When**: The same batch is submitted with `sync=true`
- **Then**: The response status is 200
- **And**: The response body contains `{"status": "duplicate", "batch_key": "batch-001"}`

#### Scenario: Async duplicate returns 202
- **Given**: Batch `batch-001` was already processed
- **When**: The same batch is submitted with `sync=false`
- **Then**: The response status is 202
- **And**: The response body contains `{"status": "duplicate", "batch_key": "batch-001"}`

#### Scenario: Duplicate is NOT 409
- **Given**: Batch `batch-001` was already processed
- **When**: The same batch is submitted
- **Then**: The response status is NOT 409 (Conflict)
- **And**: No error is returned to the client

---

### Requirement: Batch Status Tracking

The system SHALL track batch processing status with counts.

#### Scenario: Batch status updated on success
- **Given**: A batch is being processed
- **When**: All traces, spans, and events are successfully upserted
- **Then**: The batch status is updated to "accepted"
- **And**: `processing_completed_at` is set
- **And**: `trace_count`, `span_count`, `event_count` reflect actual counts

#### Scenario: Batch status on validation failure
- **Given**: A batch is being processed
- **When**: Validation fails (v1: reject entire batch)
- **Then**: The transaction is rolled back
- **And**: The batch record is NOT created (rolled back with transaction)

---

### Requirement: Project-Scoped Idempotency

The system SHALL scope batch_key uniqueness to the project.

#### Scenario: Same batch_key different projects
- **Given**: Project A has batch `batch-001`
- **And**: Project B does not have batch `batch-001`
- **When**: Project B submits batch `batch-001`
- **Then**: The batch is accepted for Project B
- **And**: Both projects have independent records

#### Scenario: Same batch_key same project
- **Given**: Project A has batch `batch-001`
- **When**: Project A submits batch `batch-001` again
- **Then**: Duplicate is detected
- **And**: Success with `status: "duplicate"` is returned

---

## Implementation Notes

### SQL Pattern for Batch Claiming

```sql
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'processing')
ON CONFLICT (project_id, batch_key) DO NOTHING
RETURNING id;
```

- If `RETURNING` returns a row → new batch, proceed
- If `RETURNING` returns no rows → duplicate, rollback immediately

### Unique Constraint

```sql
UNIQUE(project_id, batch_key)
```

---

## Related Capabilities

- [ingestion](../ingestion/spec.md) - Uses idempotency for batch processing
- [data-model](../data-model/spec.md) - Defines ingest_batches table
