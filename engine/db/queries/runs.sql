-- name: CreateRun :one
INSERT INTO engine.runs (
    project_id,
    instance_id,
    run_number,
    definition_version,
    ready_at
)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRun :one
SELECT *
FROM engine.runs
WHERE id = $1;

-- name: ListRunsByInstance :many
SELECT *
FROM engine.runs
WHERE instance_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpdateRunStatus :one
UPDATE engine.runs
SET status = $2,
    last_error_code = $3,
    last_error_message = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ClaimNextRun :one
UPDATE engine.runs
SET status = 'running',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    attempt_count = attempt_count + 1,
    updated_at = NOW()
WHERE id = (
    SELECT id
    FROM engine.runs
    WHERE (status = 'queued' AND ready_at <= NOW())
       OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW())
    ORDER BY ready_at ASC, id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;
