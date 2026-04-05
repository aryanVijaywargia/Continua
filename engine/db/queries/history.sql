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
