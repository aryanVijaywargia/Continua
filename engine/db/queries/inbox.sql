-- name: CreateInboxItem :one
INSERT INTO engine.inbox (
    project_id,
    instance_id,
    run_id,
    history_id,
    kind,
    payload,
    available_at,
    dedupe_key
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ClaimNextInboxItem :one
UPDATE engine.inbox
SET status = 'claimed',
    claimed_by = sqlc.arg(claimed_by),
    claimed_at = NOW(),
    lease_expires_at = NOW() + (sqlc.arg(lease_duration_micros)::bigint * INTERVAL '1 microsecond'),
    updated_at = NOW()
WHERE id = (
    SELECT id
    FROM engine.inbox
    WHERE (status = 'pending' AND available_at <= NOW())
       OR (status = 'claimed' AND lease_expires_at IS NOT NULL AND lease_expires_at < NOW())
    ORDER BY available_at ASC, ordinal ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: ListPendingInboxByRun :many
SELECT *
FROM engine.inbox
WHERE run_id = $1
  AND status = 'pending'
  AND available_at <= NOW()
ORDER BY available_at ASC, ordinal ASC;

-- name: CountOpenInboxByRun :one
SELECT COUNT(*)
FROM engine.inbox
WHERE run_id = $1
  AND status IN ('pending', 'claimed')
  AND kind <> 'cancel';

-- name: ListOpenInboxItemsByRunAndKind :many
SELECT *
FROM engine.inbox
WHERE run_id = $1
  AND kind = $2
  AND status IN ('pending', 'claimed')
ORDER BY available_at ASC, ordinal ASC;

-- name: ListDiscardedTimerInboxItemsByRun :many
SELECT *
FROM engine.inbox
WHERE run_id = $1
  AND kind = 'timer'
  AND status = 'discarded'
ORDER BY available_at ASC, ordinal ASC;

-- name: ListDueTimerRunIDs :many
SELECT DISTINCT run_id
FROM engine.inbox
WHERE kind = 'timer'
  AND run_id IS NOT NULL
  AND status = 'pending'
  AND available_at <= NOW()
ORDER BY run_id ASC;

-- name: ListDueTimerRunIDsByProject :many
SELECT DISTINCT run_id
FROM engine.inbox
WHERE project_id = $1
  AND kind = 'timer'
  AND run_id IS NOT NULL
  AND status = 'pending'
  AND available_at <= NOW()
ORDER BY run_id ASC;

-- name: MarkInboxProcessed :one
UPDATE engine.inbox
SET status = 'processed',
    resolved_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
  AND status = 'pending'
RETURNING *;

-- name: MarkInboxDiscarded :one
UPDATE engine.inbox
SET status = 'discarded',
    resolved_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DiscardOpenInboxItemsByRun :many
UPDATE engine.inbox
SET status = 'discarded',
    resolved_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    lease_expires_at = NULL,
    updated_at = NOW()
WHERE run_id = $1
  AND status IN ('pending', 'claimed')
RETURNING *;

-- name: ClearInboxHistoryByRun :execrows
UPDATE engine.inbox
SET history_id = NULL,
    updated_at = NOW()
WHERE run_id = $1
  AND history_id IS NOT NULL;
