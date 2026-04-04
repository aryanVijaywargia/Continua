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

-- name: GetRequestDedupeByScopeAndKey :one
SELECT *
FROM engine.request_dedupe
WHERE project_id = $1
  AND request_scope = $2
  AND request_key = $3;

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
