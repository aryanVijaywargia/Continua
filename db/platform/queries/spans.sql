-- name: GetSpan :one
SELECT * FROM spans WHERE id = $1;

-- name: ListSpansByTrace :many
SELECT * FROM spans
WHERE trace_id = $1
ORDER BY started_at ASC;

-- name: CreateSpan :one
INSERT INTO spans (id, trace_id, parent_span_id, name, kind, status, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateSpanStatus :one
UPDATE spans
SET status = $2, ended_at = $3, error_message = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateSpanMetrics :one
UPDATE spans
SET tokens_in = $2, tokens_out = $3, cost_usd = $4, latency_ms = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;
