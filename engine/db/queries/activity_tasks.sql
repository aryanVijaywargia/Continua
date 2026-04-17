-- name: CreateActivityTask :one
INSERT INTO engine.activity_tasks (
    project_id,
    instance_id,
    run_id,
    history_id,
    activity_key,
    activity_type,
    input,
    available_at,
    execution_target,
    max_attempts,
    initial_backoff_ms,
    max_backoff_ms,
    backoff_multiplier
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetActivityTask :one
SELECT *
FROM engine.activity_tasks
WHERE id = $1;

-- name: GetActivityTaskByRunAndKey :one
SELECT *
FROM engine.activity_tasks
WHERE run_id = $1
  AND activity_key = $2;

-- name: ListActivityTasksByRun :many
SELECT *
FROM engine.activity_tasks
WHERE run_id = $1
ORDER BY created_at ASC, id ASC;

-- name: CountOpenActivityTasksByRun :one
SELECT COUNT(*)
FROM engine.activity_tasks
WHERE run_id = $1
  AND status IN ('queued', 'claimed');

-- name: ListOpenActivityTasksByRun :many
SELECT *
FROM engine.activity_tasks
WHERE run_id = $1
  AND status IN ('queued', 'claimed')
ORDER BY available_at ASC, id ASC;

-- name: ListCancelledActivityTasksByRun :many
SELECT *
FROM engine.activity_tasks
WHERE run_id = $1
  AND status = 'cancelled'
ORDER BY available_at ASC, id ASC;

-- name: ClaimNextActivityTask :one
UPDATE engine.activity_tasks
SET status = 'claimed',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    lease_duration_ms = sqlc.arg(lease_duration_micros)::bigint / 1000,
    attempt_count = attempt_count + 1,
    updated_at = NOW()
WHERE id = (
    SELECT id
    FROM engine.activity_tasks
    WHERE execution_target = 'local'
      AND ((status = 'queued' AND available_at <= NOW())
        OR (status = 'claimed' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW()))
    ORDER BY available_at ASC, id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: ClaimNextActivityTaskByProject :one
UPDATE engine.activity_tasks
SET status = 'claimed',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    lease_duration_ms = sqlc.arg(lease_duration_micros)::bigint / 1000,
    attempt_count = attempt_count + 1,
    updated_at = NOW()
WHERE id = (
    SELECT candidate.id
    FROM engine.activity_tasks AS candidate
    WHERE candidate.project_id = sqlc.arg(project_filter_id)
      AND candidate.execution_target = 'local'
      AND ((candidate.status = 'queued' AND candidate.available_at <= NOW())
        OR (candidate.status = 'claimed' AND candidate.lease_expires_at IS NOT NULL AND candidate.lease_expires_at < NOW()))
    ORDER BY candidate.available_at ASC, candidate.id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: ClaimRemoteActivityTasks :many
WITH candidates AS (
    SELECT candidate.id
    FROM engine.activity_tasks AS candidate
    WHERE candidate.project_id = sqlc.arg(project_filter_id)
      AND candidate.execution_target = 'remote'
      AND candidate.activity_type = ANY(sqlc.arg(activity_types)::text[])
      AND ((candidate.status = 'queued' AND candidate.available_at <= NOW())
        OR (candidate.status = 'claimed' AND candidate.lease_expires_at IS NOT NULL AND candidate.lease_expires_at < NOW()))
    ORDER BY candidate.available_at ASC, candidate.id ASC
    LIMIT sqlc.arg(max_tasks)::int
    FOR UPDATE SKIP LOCKED
)
UPDATE engine.activity_tasks AS task
SET status = 'claimed',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_ms)::bigint * INTERVAL '1 millisecond'),
    lease_duration_ms = sqlc.arg(lease_duration_ms),
    attempt_count = task.attempt_count + 1,
    updated_at = NOW()
FROM candidates
WHERE task.id = candidates.id
RETURNING task.*;

-- name: GetActivityTaskRemoteConflictState :one
SELECT id, project_id, execution_target, status, claimed_by, lease_expires_at, completed_at
FROM engine.activity_tasks
WHERE id = $1
  AND project_id = $2;

-- name: GetActivityTaskByProjectForUpdate :one
SELECT *
FROM engine.activity_tasks
WHERE id = $1
  AND project_id = $2
FOR UPDATE;

-- name: HeartbeatRemoteActivityTask :one
UPDATE engine.activity_tasks
SET lease_expires_at = NOW() + (lease_duration_ms * INTERVAL '1 millisecond'),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND project_id = sqlc.arg(project_id)
  AND execution_target = 'remote'
  AND status = 'claimed'
  AND claimed_by = sqlc.arg(claimed_by)
  AND lease_duration_ms IS NOT NULL
  AND lease_duration_ms > 0
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at > NOW()
RETURNING *;

-- name: CompleteRemoteActivityTask :one
UPDATE engine.activity_tasks
SET status = 'completed',
    output = sqlc.arg(output),
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND project_id = sqlc.arg(project_id)
  AND execution_target = 'remote'
  AND status = 'claimed'
  AND claimed_by = sqlc.arg(claimed_by)
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at > NOW()
RETURNING *;

-- name: RetryRemoteActivityTask :one
UPDATE engine.activity_tasks
SET status = 'queued',
    available_at = NOW() + (sqlc.arg(retry_delay_ms)::bigint * INTERVAL '1 millisecond'),
    last_error_code = sqlc.arg(last_error_code),
    last_error_message = sqlc.arg(last_error_message),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND project_id = sqlc.arg(project_id)
  AND execution_target = 'remote'
  AND status = 'claimed'
  AND claimed_by = sqlc.arg(claimed_by)
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at > NOW()
RETURNING *;

-- name: FailRemoteActivityTask :one
UPDATE engine.activity_tasks
SET status = 'failed',
    last_error_code = sqlc.arg(last_error_code),
    last_error_message = sqlc.arg(last_error_message),
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND project_id = sqlc.arg(project_id)
  AND execution_target = 'remote'
  AND status = 'claimed'
  AND claimed_by = sqlc.arg(claimed_by)
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at > NOW()
RETURNING *;

-- name: CompleteActivityTask :one
UPDATE engine.activity_tasks
SET status = 'completed',
    output = $3,
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'claimed'
  AND claimed_by = $2
RETURNING *;

-- name: FailActivityTask :one
UPDATE engine.activity_tasks
SET status = 'failed',
    last_error_code = $3,
    last_error_message = $4,
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'claimed'
  AND claimed_by = $2
RETURNING *;

-- name: RetryActivityTask :one
UPDATE engine.activity_tasks
SET status = 'queued',
    available_at = NOW() + (sqlc.arg(retry_delay_ms)::bigint * INTERVAL '1 millisecond'),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND status = 'claimed'
  AND claimed_by = sqlc.arg(claimed_by)
RETURNING *;

-- name: CancelOpenActivityTasksByRun :many
UPDATE engine.activity_tasks
SET status = 'cancelled',
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE run_id = $1
  AND status IN ('queued', 'claimed')
RETURNING *;

-- name: ClearActivityTaskHistoryByRun :execrows
UPDATE engine.activity_tasks
SET history_id = NULL,
    updated_at = NOW()
WHERE run_id = $1
  AND history_id IS NOT NULL;
