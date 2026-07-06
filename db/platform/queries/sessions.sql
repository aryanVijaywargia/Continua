-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: GetSessionWithTraceCount :one
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE s.id = sqlc.arg(id)
  AND (sqlc.narg(project_filter_id)::uuid IS NULL OR s.project_id = sqlc.narg(project_filter_id)::uuid);

-- name: ListSessions :many
SELECT * FROM sessions
WHERE project_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListSessionsWithTraceCount :many
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE (sqlc.narg(project_filter_id)::uuid IS NULL OR s.project_id = sqlc.narg(project_filter_id)::uuid)
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountSessions :one
SELECT COUNT(*) FROM sessions
WHERE (sqlc.narg(project_filter_id)::uuid IS NULL OR project_id = sqlc.narg(project_filter_id)::uuid);

-- name: CreateSession :one
INSERT INTO sessions (project_id, external_id, name, user_id, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateSession :one
UPDATE sessions
SET name = $2, metadata = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetOrCreateSessionByExternalID :one
-- Upsert a session by (project_id, external_id). Creates if not exists, refreshes updated_at if exists.
INSERT INTO sessions (project_id, external_id)
VALUES ($1, $2)
ON CONFLICT (project_id, external_id) DO UPDATE SET updated_at = NOW()
RETURNING *;
