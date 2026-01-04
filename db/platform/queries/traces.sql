-- name: GetTrace :one
SELECT * FROM traces WHERE id = $1;

-- name: ListTraces :many
SELECT * FROM traces
ORDER BY started_at DESC
LIMIT $1 OFFSET $2;

-- name: ListTracesBySession :many
SELECT * FROM traces
WHERE session_id = $1
ORDER BY started_at DESC;

-- name: CreateTrace :one
INSERT INTO traces (id, session_id, name, status, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateTraceStatus :one
UPDATE traces
SET status = $2, ended_at = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateTraceTokens :one
UPDATE traces
SET total_tokens_in = $2, total_tokens_out = $3, total_cost_usd = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;
