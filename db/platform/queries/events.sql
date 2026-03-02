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

-- name: GetSpanEvent :one
SELECT * FROM span_events WHERE id = $1;

-- name: ListSpanEventsBySpan :many
SELECT * FROM span_events
WHERE trace_id = $1 AND span_id = $2
ORDER BY COALESCE(event_ts, server_ingested_at) ASC, sequence NULLS LAST;

-- name: ListSpanEventsByTrace :many
SELECT * FROM span_events
WHERE trace_id = $1
ORDER BY COALESCE(event_ts, server_ingested_at) ASC, sequence NULLS LAST;

-- name: CountOrphanEvents :one
-- Returns count of events whose span_id doesn't exist in spans table
SELECT COUNT(*) FROM span_events e
LEFT JOIN spans s ON s.trace_id = e.trace_id AND s.span_id = e.span_id
WHERE e.trace_id = $1 AND s.id IS NULL;

-- name: ListOrphanEvents :many
-- Returns events whose span_id doesn't exist in spans table
SELECT e.* FROM span_events e
LEFT JOIN spans s ON s.trace_id = e.trace_id AND s.span_id = e.span_id
WHERE e.trace_id = $1 AND s.id IS NULL
ORDER BY COALESCE(e.event_ts, e.server_ingested_at) ASC;
