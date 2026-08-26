-- name: InsertSpanEvent :one
WITH inserted AS (
    INSERT INTO span_events (
        project_id, trace_id, span_id, event_type, level,
        event_ts, sequence, message, payload,
        truncated, original_size_bytes, truncation_reason,
        idempotency_key
    )
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
    ON CONFLICT (project_id, idempotency_key) WHERE idempotency_key IS NOT NULL
    DO NOTHING
    RETURNING id
)
SELECT id FROM inserted
UNION ALL
SELECT id
FROM span_events
WHERE project_id = $1
  AND idempotency_key = $13
  AND $13 IS NOT NULL
LIMIT 1;

-- name: ListSpanEventsByTrace :many
SELECT * FROM span_events
WHERE trace_id = sqlc.arg(trace_id)
  AND (sqlc.narg(project_filter_id)::uuid IS NULL OR project_id = sqlc.narg(project_filter_id)::uuid)
ORDER BY COALESCE(event_ts, server_ingested_at) ASC, sequence NULLS LAST;
