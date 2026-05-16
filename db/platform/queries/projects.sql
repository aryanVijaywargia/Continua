-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectByAPIKey :one
SELECT * FROM projects WHERE api_key_hash = $1;

-- name: ListProjects :many
SELECT * FROM projects
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountProjects :one
SELECT COUNT(*) FROM projects;

-- name: CreateProject :one
INSERT INTO projects (name, api_key_hash)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateProject :one
UPDATE projects
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: RotateProjectAPIKey :one
UPDATE projects
SET api_key_hash = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = $1;
