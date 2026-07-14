-- name: CreateRequestDedupe :one
INSERT INTO engine.request_dedupe (
    project_id,
    request_scope,
    request_key,
    instance_id,
    run_id,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateStartRequestDedupeClaim :one
INSERT INTO engine.request_dedupe (
    project_id,
    request_scope,
    request_key,
    expires_at
)
VALUES ($1, $2, $3, $4)
ON CONFLICT (project_id, request_scope, request_key) DO NOTHING
RETURNING *;

-- name: GetRequestDedupeByScopeAndKey :one
SELECT *
FROM engine.request_dedupe
WHERE project_id = $1
  AND request_scope = $2
  AND request_key = $3;

-- name: GetRequestDedupeByScopeAndKeyForUpdate :one
SELECT *
FROM engine.request_dedupe
WHERE project_id = $1
  AND request_scope = $2
  AND request_key = $3
FOR UPDATE;

-- name: RenewRequestDedupeClaim :one
UPDATE engine.request_dedupe
SET status = 'in_progress',
    instance_id = NULL,
    run_id = NULL,
    response_payload = NULL,
    error_code = NULL,
    error_message = NULL,
    expires_at = $2,
    finalized_at = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: FinalizeRequestDedupeWithResponse :one
UPDATE engine.request_dedupe
SET status = 'completed',
    response_payload = $2,
    error_code = NULL,
    error_message = NULL,
    finalized_at = NOW(),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: FinalizeRequestDedupeWithError :one
UPDATE engine.request_dedupe
SET status = 'failed',
    response_payload = NULL,
    error_code = $2,
    error_message = $3,
    finalized_at = NOW(),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ExpireRequestDedupe :execrows
UPDATE engine.request_dedupe
SET status = 'expired',
    updated_at = NOW()
WHERE status = 'in_progress'
  AND expires_at < NOW();

-- name: ReapRequestDedupe :execrows
DELETE FROM engine.request_dedupe AS target
WHERE target.id IN (
    SELECT candidate.id
    FROM engine.request_dedupe AS candidate
    WHERE (sqlc.narg(project_filter)::uuid IS NULL OR candidate.project_id = sqlc.narg(project_filter)::uuid)
      AND candidate.status IN ('completed', 'failed', 'expired')
      AND candidate.expires_at < sqlc.arg(cutoff)
    LIMIT sqlc.arg(batch_size)
);
