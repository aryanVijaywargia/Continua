-- name: GetProjectionCheckpoint :one
SELECT *
FROM engine.projection_checkpoints
WHERE projection_name = $1
  AND scope_key = $2;

-- name: AdvanceProjectionCheckpoint :one
INSERT INTO engine.projection_checkpoints (
    projection_name,
    scope_key,
    last_history_id
)
VALUES ($1, $2, $3)
ON CONFLICT (projection_name, scope_key) DO UPDATE
SET last_history_id = EXCLUDED.last_history_id,
    updated_at = NOW()
WHERE engine.projection_checkpoints.last_history_id <= EXCLUDED.last_history_id
RETURNING *;
