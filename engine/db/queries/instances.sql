-- name: CreateInstance :one
INSERT INTO engine.instances (
    project_id,
    instance_key,
    definition_name,
    metadata
)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetInstance :one
SELECT *
FROM engine.instances
WHERE id = $1;

-- name: GetInstanceByProjectAndKey :one
SELECT *
FROM engine.instances
WHERE project_id = $1
  AND instance_key = $2;

-- name: ListInstancesByKey :many
SELECT *
FROM engine.instances
WHERE instance_key = $1
ORDER BY created_at DESC, id DESC
LIMIT 2;

-- name: ListInstancesByProject :many
SELECT *
FROM engine.instances
WHERE project_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpdateInstanceStatus :one
UPDATE engine.instances
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;
