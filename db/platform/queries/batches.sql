-- name: ClaimBatch :one
-- Claims a batch for processing. Returns the batch ID if successful.
-- If batch already exists (duplicate), returns no rows.
INSERT INTO ingest_batches (project_id, batch_key, status)
VALUES ($1, $2, 'processing')
ON CONFLICT (project_id, batch_key) DO NOTHING
RETURNING id;

-- name: GetBatch :one
SELECT * FROM ingest_batches WHERE id = $1;

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

-- name: ListBatches :many
SELECT * FROM ingest_batches
WHERE project_id = $1
ORDER BY server_received_at DESC
LIMIT $2 OFFSET $3;
