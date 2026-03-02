-- name: GetSpan :one
SELECT * FROM spans WHERE id = $1;

-- name: GetSpanByExternalID :one
SELECT * FROM spans WHERE trace_id = $1 AND span_id = $2;

-- name: ListSpansByTrace :many
SELECT * FROM spans
WHERE trace_id = $1
ORDER BY COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST;

-- name: ListSpansSummaryByTrace :many
SELECT id, project_id, trace_id, span_id, parent_span_id, name, type, status,
       start_time, end_time, duration_ms, model, total_tokens, total_cost,
       input_truncated, output_truncated, depth
FROM spans
WHERE trace_id = $1
ORDER BY COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST;

-- name: CountSpansByTrace :one
SELECT COUNT(*) FROM spans WHERE trace_id = $1;

-- name: CreateSpan :one
INSERT INTO spans (
    project_id, trace_id, span_id, parent_span_id, name, type,
    status, status_message, level, start_time, end_time,
    input, input_truncated, input_original_size_bytes, input_truncation_reason,
    output, output_truncated, output_original_size_bytes, output_truncation_reason,
    model, provider, prompt_tokens, completion_tokens, total_tokens, total_cost,
    metadata, sequence, depth
)
VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19,
    $20, $21, $22, $23, $24, $25,
    $26, $27, $28
)
RETURNING *;

-- name: UpsertSpan :one
-- Upsert span with patch semantics: NULL values don't overwrite existing.
-- Status is protected: 'failed'/'error' status can never be downgraded.
INSERT INTO spans (
    project_id, trace_id, span_id, parent_span_id, name, type,
    status, status_message, level, start_time, end_time,
    input, input_truncated, input_original_size_bytes, input_truncation_reason,
    output, output_truncated, output_original_size_bytes, output_truncation_reason,
    model, provider, prompt_tokens, completion_tokens, total_tokens, total_cost,
    metadata, sequence, depth
)
VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19,
    $20, $21, $22, $23, $24, $25,
    $26, $27, $28
)
ON CONFLICT (trace_id, span_id) DO UPDATE SET
    parent_span_id = COALESCE(EXCLUDED.parent_span_id, spans.parent_span_id),
    name = COALESCE(EXCLUDED.name, spans.name),
    type = COALESCE(EXCLUDED.type, spans.type),
    -- Status protection: never downgrade from failed/error
    status = CASE
        WHEN spans.status IN ('failed', 'error') THEN spans.status
        ELSE COALESCE(EXCLUDED.status, spans.status)
    END,
    status_message = COALESCE(EXCLUDED.status_message, spans.status_message),
    level = COALESCE(EXCLUDED.level, spans.level),
    -- Use LEAST/GREATEST for time merging to handle out-of-order updates correctly
    start_time = COALESCE(
        LEAST(spans.start_time, EXCLUDED.start_time),
        spans.start_time,
        EXCLUDED.start_time
    ),
    end_time = COALESCE(
        GREATEST(spans.end_time, EXCLUDED.end_time),
        spans.end_time,
        EXCLUDED.end_time
    ),
    input = COALESCE(EXCLUDED.input, spans.input),
    input_truncated = COALESCE(EXCLUDED.input_truncated, spans.input_truncated),
    input_original_size_bytes = COALESCE(EXCLUDED.input_original_size_bytes, spans.input_original_size_bytes),
    input_truncation_reason = COALESCE(EXCLUDED.input_truncation_reason, spans.input_truncation_reason),
    output = COALESCE(EXCLUDED.output, spans.output),
    output_truncated = COALESCE(EXCLUDED.output_truncated, spans.output_truncated),
    output_original_size_bytes = COALESCE(EXCLUDED.output_original_size_bytes, spans.output_original_size_bytes),
    output_truncation_reason = COALESCE(EXCLUDED.output_truncation_reason, spans.output_truncation_reason),
    model = COALESCE(EXCLUDED.model, spans.model),
    provider = COALESCE(EXCLUDED.provider, spans.provider),
    prompt_tokens = COALESCE(EXCLUDED.prompt_tokens, spans.prompt_tokens),
    completion_tokens = COALESCE(EXCLUDED.completion_tokens, spans.completion_tokens),
    total_tokens = COALESCE(EXCLUDED.total_tokens, spans.total_tokens),
    total_cost = COALESCE(EXCLUDED.total_cost, spans.total_cost),
    metadata = CASE
        WHEN EXCLUDED.metadata IS NOT NULL THEN spans.metadata || EXCLUDED.metadata
        ELSE spans.metadata
    END,
    sequence = COALESCE(EXCLUDED.sequence, spans.sequence),
    depth = COALESCE(EXCLUDED.depth, spans.depth),
    duration_ms = CASE
        WHEN COALESCE(
            GREATEST(spans.end_time, EXCLUDED.end_time),
            spans.end_time,
            EXCLUDED.end_time
        ) IS NOT NULL
        AND COALESCE(
            LEAST(spans.start_time, EXCLUDED.start_time),
            spans.start_time,
            EXCLUDED.start_time
        ) IS NOT NULL
        THEN EXTRACT(EPOCH FROM (
            COALESCE(
                GREATEST(spans.end_time, EXCLUDED.end_time),
                spans.end_time,
                EXCLUDED.end_time
            ) - COALESCE(
                LEAST(spans.start_time, EXCLUDED.start_time),
                spans.start_time,
                EXCLUDED.start_time
            )
        )) * 1000
        ELSE spans.duration_ms
    END,
    updated_at = NOW(),
    version = spans.version + 1
RETURNING *;

-- name: UpdateSpanStatus :one
UPDATE spans
SET status = $2, end_time = $3, status_message = $4,
    duration_ms = CASE
        WHEN $3 IS NOT NULL THEN EXTRACT(EPOCH FROM ($3 - start_time)) * 1000
        ELSE duration_ms
    END,
    updated_at = NOW(), version = version + 1
WHERE id = $1
RETURNING *;

-- name: UpdateSpanOutput :one
UPDATE spans
SET output = $2, output_truncated = $3, output_original_size_bytes = $4,
    output_truncation_reason = $5, updated_at = NOW(), version = version + 1
WHERE id = $1
RETURNING *;

-- name: UpdateSpanTokens :one
UPDATE spans
SET prompt_tokens = $2, completion_tokens = $3, total_tokens = $4,
    total_cost = $5, updated_at = NOW(), version = version + 1
WHERE id = $1
RETURNING *;
