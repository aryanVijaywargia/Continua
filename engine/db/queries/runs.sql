-- name: CreateRun :one
INSERT INTO engine.runs (
    project_id,
    instance_id,
    run_number,
    definition_version,
    ready_at,
    continued_from_run_id
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRun :one
SELECT *
FROM engine.runs
WHERE id = $1;

-- name: GetRunByProjectAndID :one
SELECT *
FROM engine.runs
WHERE project_id = $1
  AND id = $2;

-- name: GetRunForUpdate :one
SELECT *
FROM engine.runs
WHERE id = $1
FOR UPDATE;

-- name: GetRunByProjectAndIDForUpdate :one
SELECT *
FROM engine.runs
WHERE project_id = $1
  AND id = $2
FOR UPDATE;

-- name: ListRunsByInstance :many
SELECT *
FROM engine.runs
WHERE instance_id = $1
ORDER BY run_number DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: GetLatestRunByInstance :one
SELECT *
FROM engine.runs
WHERE instance_id = $1
ORDER BY run_number DESC, id DESC
LIMIT 1;

-- name: UpdateRunStatus :one
UPDATE engine.runs
SET status = $2,
    last_error_code = $3,
    last_error_message = $4,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: TransitionRunToWaiting :one
UPDATE engine.runs
SET status = 'waiting',
    waiting_for = $3,
    custom_status = $4,
    result = NULL,
    completed_at = NULL,
    last_error_code = NULL,
    last_error_message = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'running'
  AND claimed_by = $2
RETURNING *;

-- name: TransitionRunToCompleted :one
UPDATE engine.runs
SET status = 'completed',
    result = $3,
    custom_status = $4,
    waiting_for = NULL,
    completed_at = NOW(),
    last_error_code = NULL,
    last_error_message = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'running'
  AND claimed_by = $2
RETURNING *;

-- name: TransitionRunToFailed :one
UPDATE engine.runs
SET status = 'failed',
    result = NULL,
    custom_status = $3,
    waiting_for = NULL,
    completed_at = NOW(),
    last_error_code = $4,
    last_error_message = $5,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'running'
  AND claimed_by = $2
RETURNING *;

-- name: TransitionRunToCancelled :one
UPDATE engine.runs
SET status = 'cancelled',
    result = NULL,
    custom_status = $2,
    waiting_for = NULL,
    completed_at = NOW(),
    last_error_code = 'cancelled',
    last_error_message = 'workflow cancelled',
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'running'
RETURNING *;

-- name: TransitionRunToContinuedAsNew :one
UPDATE engine.runs
SET status = 'continued_as_new',
    result = NULL,
    custom_status = $4,
    waiting_for = NULL,
    completed_at = NOW(),
    last_error_code = NULL,
    last_error_message = NULL,
    continued_to_run_id = $3,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'running'
  AND claimed_by = $2
RETURNING *;

-- name: TransitionRunToTerminated :one
UPDATE engine.runs
SET status = 'terminated',
    result = NULL,
    waiting_for = NULL,
    completed_at = NOW(),
    last_error_code = 'terminated',
    last_error_message = 'run terminated by operator',
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status IN ('queued', 'running', 'waiting', 'suspended')
RETURNING *;

-- name: TransitionRunToSuspended :one
UPDATE engine.runs
SET status = 'suspended',
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status IN ('queued', 'waiting')
RETURNING *;

-- name: TransitionRunToQueuedFromSuspended :one
UPDATE engine.runs
SET status = 'queued',
    waiting_for = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    ready_at = NOW(),
    updated_at = NOW()
WHERE id = $1
  AND status = 'suspended'
RETURNING *;

-- name: WakeWaitingRun :one
UPDATE engine.runs
SET status = 'queued',
    waiting_for = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    ready_at = NOW(),
    updated_at = NOW()
WHERE id = $1
  AND status = 'waiting'
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
