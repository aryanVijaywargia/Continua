-- name: CreateRun :one
WITH new_run AS (
    SELECT gen_random_uuid() AS id
)
INSERT INTO engine.runs (
    id,
    project_id,
    instance_id,
    run_number,
    definition_version,
    ready_at,
    continued_from_run_id,
    parent_run_id,
    root_run_id,
    child_key,
    child_depth
)
SELECT id, $1, $2, $3, $4, $5, $6, NULL, id, NULL, 0
FROM new_run
RETURNING *;

-- name: CreateChildRun :one
INSERT INTO engine.runs (
    project_id,
    instance_id,
    run_number,
    definition_version,
    ready_at,
    continued_from_run_id,
    parent_run_id,
    root_run_id,
    child_key,
    child_depth
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
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
-- Run coordination locks (this and GetRunByProjectAndIDForUpdate) take FOR NO KEY UPDATE:
-- they serialize run writers, but do not block the FOR KEY SHARE that a foreign key check
-- takes on a run that a new row names. No runtime path changes a run's keys or deletes it.
SELECT *
FROM engine.runs
WHERE id = $1
FOR NO KEY UPDATE;

-- name: GetRunByProjectAndIDForUpdate :one
SELECT *
FROM engine.runs
WHERE project_id = $1
  AND id = $2
FOR NO KEY UPDATE;

-- name: ListRetainableTerminalRunIDs :many
SELECT r.id
FROM engine.runs AS r
JOIN public.traces AS t ON t.engine_run_id = r.id
WHERE (sqlc.narg(project_filter)::uuid IS NULL OR r.project_id = sqlc.narg(project_filter)::uuid)
  AND r.status IN ('completed', 'failed', 'cancelled', 'terminated')
  AND r.completed_at IS NOT NULL
  AND r.completed_at < sqlc.arg(cutoff)::timestamptz
  AND t.engine_latest_history_id IS NOT NULL
  AND t.engine_last_projected_history_id IS NOT NULL
  AND t.engine_last_projected_history_id >= t.engine_latest_history_id
  AND COALESCE(t.engine_projection_state, '') <> 'journal_expired'
ORDER BY r.completed_at ASC
LIMIT sqlc.arg(batch_size);

-- name: GetRetainableTerminalRunForUpdate :one
SELECT r.id
FROM engine.runs AS r
JOIN public.traces AS t ON t.engine_run_id = r.id
WHERE r.id = sqlc.arg(run_id)
  AND (sqlc.narg(project_filter)::uuid IS NULL OR r.project_id = sqlc.narg(project_filter)::uuid)
  AND r.status IN ('completed', 'failed', 'cancelled', 'terminated')
  AND t.engine_latest_history_id IS NOT NULL
  AND t.engine_last_projected_history_id IS NOT NULL
  AND t.engine_last_projected_history_id >= t.engine_latest_history_id
  AND COALESCE(t.engine_projection_state, '') <> 'journal_expired'
FOR NO KEY UPDATE OF r
FOR UPDATE OF t;

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

-- name: TransitionRunToQuarantined :one
UPDATE engine.runs
SET status = 'quarantined',
    waiting_for = $3,
    last_error_code = $4,
    last_error_message = $5,
    result = NULL,
    completed_at = NULL,
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
  AND status IN ('queued', 'running', 'waiting', 'suspended', 'quarantined')
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

-- name: TransitionRunToQueuedFromQuarantined :one
UPDATE engine.runs
SET status = 'queued',
    waiting_for = NULL,
    last_error_code = NULL,
    last_error_message = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    ready_at = NOW(),
    updated_at = NOW()
WHERE id = $1
  AND status = 'quarantined'
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

-- name: WakeWaitingChildWorkflowRun :one
UPDATE engine.runs
SET status = 'queued',
    waiting_for = NULL,
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    ready_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
  AND status = 'waiting'
  AND waiting_for @> jsonb_build_object('kind', 'child_workflow', 'child_key', sqlc.arg(child_key)::text)
RETURNING *;

-- name: ClaimNextRun :one
-- A claim appends no history row, so the projector never sees it. The statement
-- mirrors the new status onto the run's trace itself (#347): next_run takes the
-- run lock (SKIP LOCKED) before the trace write, which touches one trace row.
-- The write is unconditional: a status filter would read the statement snapshot,
-- so a release that commits before the run lock would leave the trace queued.
WITH next_run AS (
    SELECT id
    FROM engine.runs
    WHERE (status = 'queued' AND ready_at <= NOW())
       OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW())
    ORDER BY ready_at ASC, id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
), projected AS (
    UPDATE public.traces
    SET engine_run_status = 'running',
        updated_at = NOW(),
        version = COALESCE(version, 1) + 1
    WHERE engine_run_id = (SELECT id FROM next_run)
)
UPDATE engine.runs
SET status = 'running',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    attempt_count = attempt_count + 1,
    updated_at = NOW()
WHERE id = (SELECT id FROM next_run)
RETURNING *;

-- name: ClaimNextRunByProject :one
-- Same as ClaimNextRun, limited to one project.
WITH next_run AS (
    SELECT candidate.id
    FROM engine.runs AS candidate
    WHERE candidate.project_id = sqlc.arg(project_filter_id)
      AND ((candidate.status = 'queued' AND candidate.ready_at <= NOW())
        OR (candidate.status = 'running' AND candidate.lease_expires_at IS NOT NULL AND candidate.lease_expires_at < NOW()))
    ORDER BY candidate.ready_at ASC, candidate.id ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
), projected AS (
    UPDATE public.traces
    SET engine_run_status = 'running',
        updated_at = NOW(),
        version = COALESCE(version, 1) + 1
    WHERE engine_run_id = (SELECT id FROM next_run)
)
UPDATE engine.runs
SET status = 'running',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    attempt_count = attempt_count + 1,
    updated_at = NOW()
WHERE id = (SELECT id FROM next_run)
RETURNING *;

-- name: ReleaseRunsByClaimant :many
UPDATE engine.runs
SET status = 'queued',
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE claimed_by = $1
  AND status = 'running'
RETURNING *;

-- name: LockRunTraces :exec
-- Lock the projected traces of several runs in platform ingest's trace lock
-- order (project, then external trace id in byte order), as the platform query
-- LockEngineRunTraces does. Take it after the run locks and before any trace
-- write, so a concurrent ingest batch queues instead of deadlocking.
SELECT id
FROM public.traces
WHERE engine_run_id = ANY(sqlc.arg(run_ids)::uuid[])
ORDER BY project_id, trace_id COLLATE "C"
FOR NO KEY UPDATE;

-- name: SyncProjectedRunStatuses :exec
-- Mirror engine.runs.status onto the projected traces of the given runs.
UPDATE public.traces AS t
SET engine_run_status = r.status::text,
    updated_at = NOW(),
    version = COALESCE(t.version, 1) + 1
FROM engine.runs AS r
WHERE r.id = t.engine_run_id
  AND t.engine_run_id = ANY(sqlc.arg(run_ids)::uuid[]);
