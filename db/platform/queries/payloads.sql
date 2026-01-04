-- name: GetPayload :one
SELECT * FROM payloads WHERE id = $1;

-- name: GetPayloadsBySpan :many
SELECT * FROM payloads
WHERE span_id = $1
ORDER BY created_at ASC;

-- name: CreatePayload :one
INSERT INTO payloads (id, span_id, direction, content_type, body, truncated, original_size)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
