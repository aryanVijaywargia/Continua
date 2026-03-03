-- name: GetTrace :one
SELECT * FROM traces WHERE id = $1;

-- name: GetTraceVersion :one
-- Get trace version for optimistic concurrency checks (e.g., rollup re-enqueue).
SELECT version FROM traces WHERE id = $1;

-- name: GetTraceByExternalID :one
SELECT * FROM traces WHERE project_id = $1 AND trace_id = $2;

-- name: GetTraceUUID :one
SELECT id FROM traces WHERE project_id = $1 AND trace_id = $2;

-- name: ListTraces :many
SELECT * FROM traces
WHERE project_id = $1
ORDER BY COALESCE(start_time, server_received_at) DESC
LIMIT $2 OFFSET $3;

-- name: ListTracesBySession :many
SELECT * FROM traces
WHERE project_id = $1 AND session_id = $2
ORDER BY COALESCE(start_time, server_received_at) DESC
LIMIT $3 OFFSET $4;

-- name: CountTraces :one
SELECT COUNT(*) FROM traces WHERE project_id = $1;

-- name: CountTracesBySession :one
SELECT COUNT(*) FROM traces WHERE project_id = $1 AND session_id = $2;

-- name: CreateTrace :one
INSERT INTO traces (
    project_id, session_id, trace_id, name, user_id, tags,
    environment, release, metadata, input, output,
    status, start_time, end_time
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: UpsertTrace :one
-- Upsert trace with patch semantics: NULL values don't overwrite existing.
-- Status is protected: 'failed'/'error' status can never be downgraded.
INSERT INTO traces (
    project_id, session_id, trace_id, name, user_id, tags,
    environment, release, metadata, input, output,
    status, start_time, end_time
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (project_id, trace_id) DO UPDATE SET
    session_id = COALESCE(EXCLUDED.session_id, traces.session_id),
    name = COALESCE(EXCLUDED.name, traces.name),
    user_id = COALESCE(EXCLUDED.user_id, traces.user_id),
    tags = CASE WHEN EXCLUDED.tags IS NOT NULL AND array_length(EXCLUDED.tags, 1) > 0 THEN EXCLUDED.tags ELSE traces.tags END,
    environment = COALESCE(EXCLUDED.environment, traces.environment),
    release = COALESCE(EXCLUDED.release, traces.release),
    metadata = CASE
        WHEN EXCLUDED.metadata IS NOT NULL THEN traces.metadata || EXCLUDED.metadata
        ELSE traces.metadata
    END,
    input = COALESCE(EXCLUDED.input, traces.input),
    output = COALESCE(EXCLUDED.output, traces.output),
    -- Status protection: never downgrade from failed/error
    status = CASE
        WHEN traces.status IN ('failed', 'error') THEN traces.status
        ELSE COALESCE(EXCLUDED.status, traces.status)
    END,
    start_time = COALESCE(
        LEAST(traces.start_time, EXCLUDED.start_time),
        traces.start_time,
        EXCLUDED.start_time
    ),
    end_time = COALESCE(
        GREATEST(traces.end_time, EXCLUDED.end_time),
        traces.end_time,
        EXCLUDED.end_time
    ),
    updated_at = NOW(),
    version = traces.version + 1
RETURNING *;

-- name: UpdateTraceStatus :one
UPDATE traces
SET status = $2, end_time = $3, updated_at = NOW(), version = version + 1
WHERE id = $1
RETURNING *;

-- name: UpdateTraceRollups :exec
UPDATE traces
SET
    total_spans = $2,
    total_tokens_in = $3,
    total_tokens_out = $4,
    total_cost = $5,
    error_count = $6,
    duration_ms = CASE
        WHEN end_time IS NOT NULL AND start_time IS NOT NULL
        THEN EXTRACT(EPOCH FROM (end_time - start_time)) * 1000
        ELSE duration_ms
    END,
    updated_at = NOW()
WHERE id = $1;
