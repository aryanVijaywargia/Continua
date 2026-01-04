-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: ListSessions :many
SELECT * FROM sessions
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateSession :one
INSERT INTO sessions (id, name, metadata)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateSession :one
UPDATE sessions
SET name = $2, metadata = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;
