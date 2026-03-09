-- name: ClaimBatch :one
-- Claims a batch for processing. Returns the batch ID if successful.
-- If batch already exists (duplicate), returns no rows.
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'queued')
ON CONFLICT (project_id, batch_key) DO NOTHING
RETURNING id;

-- name: ClaimBatchOrGetExisting :one
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'queued')
ON CONFLICT (project_id, batch_key) DO UPDATE
SET batch_key = EXCLUDED.batch_key
RETURNING
    id,
    project_id,
    batch_key,
    status,
    server_received_at,
    processing_started_at,
    processing_completed_at,
    trace_count,
    span_count,
    event_count,
    accepted_count,
    rejected_count,
    attempt_count,
    last_error_code,
    last_error_message,
    last_error_at,
    created_at,
    (xmax = 0) AS inserted;

-- name: GetBatch :one
SELECT * FROM ingest_batches WHERE id = $1;

-- name: GetBatchForProject :one
SELECT * FROM ingest_batches WHERE id = $1 AND project_id = $2;

-- name: GetBatchByKey :one
SELECT * FROM ingest_batches WHERE project_id = $1 AND batch_key = $2;

-- name: UpdateBatchStatus :exec
UPDATE ingest_batches
SET status = $2,
    processing_completed_at = NOW(),
    trace_count = $3,
    span_count = $4,
    event_count = $5,
    accepted_count = $6,
    rejected_count = $7
WHERE id = $1;

-- name: InsertBatchPayload :exec
INSERT INTO ingest_batch_payloads (
    batch_id,
    payload_bytes,
    compression,
    content_type,
    byte_size
)
VALUES ($1, $2, $3, $4, $5);

-- name: GetBatchPayload :one
SELECT * FROM ingest_batch_payloads WHERE batch_id = $1;

-- name: DeleteBatchPayload :exec
DELETE FROM ingest_batch_payloads WHERE batch_id = $1;

-- name: MarkBatchProcessingIfQueued :one
UPDATE ingest_batches
SET status = 'processing',
    processing_started_at = COALESCE(processing_started_at, NOW()),
    attempt_count = attempt_count + 1
WHERE id = $1
  AND status = 'queued'
RETURNING *;

-- name: MarkBatchCompleted :exec
UPDATE ingest_batches
SET status = 'completed',
    processing_completed_at = NOW(),
    trace_count = $2,
    span_count = $3,
    event_count = $4,
    accepted_count = $5,
    rejected_count = $6,
    last_error_code = NULL,
    last_error_message = NULL,
    last_error_at = NULL
WHERE id = $1;

-- name: MarkBatchFailed :exec
UPDATE ingest_batches
SET status = 'failed',
    processing_completed_at = NOW(),
    last_error_code = $2,
    last_error_message = $3,
    last_error_at = NOW()
WHERE id = $1;

-- name: MarkBatchQueued :exec
UPDATE ingest_batches
SET status = 'queued',
    processing_completed_at = NULL,
    last_error_code = $2,
    last_error_message = $3,
    last_error_at = NOW()
WHERE id = $1;

-- name: ListBatches :many
SELECT * FROM ingest_batches
WHERE project_id = $1
ORDER BY server_received_at DESC
LIMIT $2 OFFSET $3;

-- name: CleanupExpiredPayloads :many
DELETE FROM ingest_batch_payloads AS p
USING ingest_batches AS b
WHERE p.batch_id = b.id
  AND b.status = 'failed'
  AND b.processing_completed_at IS NOT NULL
  AND b.processing_completed_at < $1
RETURNING p.batch_id;
