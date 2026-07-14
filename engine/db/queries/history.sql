-- name: AppendHistory :one
INSERT INTO engine.history (
    project_id,
    instance_id,
    run_id,
    sequence_no,
    event_type,
    payload
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetHistoryByRun :many
SELECT *
FROM engine.history
WHERE run_id = $1
ORDER BY sequence_no ASC, id ASC;

-- name: GetLatestHistoryIDByRun :one
SELECT COALESCE(MAX(id), 0)::bigint
FROM engine.history
WHERE run_id = $1;

-- name: GetMaxHistorySequenceByRun :one
SELECT COALESCE(MAX(sequence_no), 0)::int
FROM engine.history
WHERE run_id = $1;

-- name: ListHistoryByRunAfterID :many
SELECT *
FROM engine.history
WHERE run_id = $1
  AND id > $2
ORDER BY id ASC
LIMIT $3;

-- name: ListHistoryByRunAfterSequence :many
SELECT *
FROM engine.history
WHERE run_id = $1
  AND sequence_no > $2
ORDER BY sequence_no ASC, id ASC
LIMIT $3;

-- name: GetHistoryByInstance :many
SELECT *
FROM engine.history
WHERE instance_id = $1
ORDER BY id ASC;

-- name: DeleteHistoryByRun :exec
DELETE FROM engine.history
WHERE run_id = $1;

-- name: DeleteHistoryByRunCounted :execrows
DELETE FROM engine.history
WHERE run_id = $1;
