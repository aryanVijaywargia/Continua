## ADDED Requirements

### Requirement: Batch Status Endpoint

The system SHALL expose `GET /v1/ingest/batches/{id}` for polling the processing status of a submitted batch, using internal-to-public status mapping.

#### Scenario: Lookup queued batch
- **WHEN** a client queries a batch that is internally `queued`
- **THEN** the response returns `status: accepted` with `batch_id`, `batch_key`, `attempt_count`, and `server_received_at`

#### Scenario: Lookup processing batch
- **WHEN** a client queries a batch that is internally `processing`
- **THEN** the response returns `status: processing` with `processing_started_at`

#### Scenario: Lookup completed batch
- **WHEN** a client queries a batch that is internally `completed`
- **THEN** the response returns `status: completed` with counts and `processing_completed_at`

#### Scenario: Lookup failed batch
- **WHEN** a client queries a batch that is internally `failed`
- **THEN** the response returns `status: failed` with `last_error_code`, `last_error_message`, and `processing_completed_at`

#### Scenario: Batch not found
- **WHEN** a client queries a batch ID that does not exist
- **THEN** the system returns `404`

#### Scenario: Project scoping
- **WHEN** a client queries a batch that belongs to a different project
- **THEN** the system returns `404`

### Requirement: Public Status Vocabulary

The batch status API SHALL use a public vocabulary that maps from internal database states without exposing implementation details.

#### Scenario: Status mapping consistency
- **WHEN** a batch is created via `POST /v1/ingest` with `status: accepted` and later queried via `GET /v1/ingest/batches/{id}`
- **THEN** both endpoints return `accepted` for the same batch in `queued` state
- **AND** the `GET` endpoint never returns `ok`

#### Scenario: Legacy accepted rows map to completed
- **WHEN** the batch status endpoint reads a legacy row with internal `status='accepted'`
- **THEN** the public response returns `status: completed`
