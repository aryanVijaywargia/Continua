-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: GetSessionWithTraceCount :one
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE s.id = $1;

-- name: ListSessions :many
SELECT * FROM sessions
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListSessionsWithTraceCount :many
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE s.project_id = $1
ORDER BY s.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountSessions :one
SELECT COUNT(*) FROM sessions WHERE project_id = $1;

-- name: CreateSession :one
INSERT INTO sessions (project_id, name, user_id, metadata)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateSession :one
UPDATE sessions
SET name = $2, metadata = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;
