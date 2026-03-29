-- name: GetSessionNarrativeSummary :one
SELECT
    s.id AS session_id,
    COUNT(t.id) AS total_trace_count,
    COUNT(t.id) FILTER (
        WHERE COALESCE(LOWER(t.status), '') NOT IN ('completed', 'ok', 'failed', 'error', 'cancelled')
    ) AS running_trace_count,
    COUNT(t.id) FILTER (
        WHERE COALESCE(LOWER(t.status), '') IN ('completed', 'ok')
    ) AS completed_trace_count,
    COUNT(t.id) FILTER (
        WHERE COALESCE(LOWER(t.status), '') IN ('failed', 'error', 'cancelled')
    ) AS failed_trace_count,
    COALESCE(SUM(t.total_tokens_in)::bigint, 0::bigint) AS total_tokens_in,
    COALESCE(SUM(t.total_tokens_out)::bigint, 0::bigint) AS total_tokens_out,
    COALESCE(SUM(t.total_cost), 0::numeric(12, 6))::numeric(12, 6) AS total_cost_usd,
    COALESCE(
        MIN(COALESCE(t.start_time, t.server_received_at))::timestamptz,
        '0001-01-01 00:00:00+00'::timestamptz
    ) AS started_at,
    COALESCE(
        MAX(
            CASE
                WHEN t.id IS NULL THEN NULL
                ELSE GREATEST(
                    t.server_received_at,
                    COALESCE(t.end_time, '1970-01-01'::timestamptz)
                )
            END
        )::timestamptz,
        '0001-01-01 00:00:00+00'::timestamptz
    ) AS last_activity_at
FROM sessions s
LEFT JOIN traces t
    ON t.session_id = s.id
   AND t.project_id = s.project_id
WHERE s.id = $1
  AND s.project_id = $2
GROUP BY s.id;

-- name: ListSessionNarrativeTraces :many
WITH capped_traces AS (
    SELECT
        t.id,
        t.trace_id,
        t.name,
        t.status,
        t.user_id,
        COALESCE(t.start_time, t.server_received_at) AS started_at,
        t.end_time AS ended_at,
        t.metadata,
        t.server_received_at,
        t.total_tokens_in,
        t.total_tokens_out,
        t.total_cost,
        t.error_count
    FROM traces t
    WHERE t.session_id = $1
      AND t.project_id = $2
    ORDER BY COALESCE(t.start_time, t.server_received_at) ASC, t.id ASC
    LIMIT $3
),
span_activity AS (
    SELECT
        s.trace_id,
        MAX(
            GREATEST(
                s.start_time,
                COALESCE(s.end_time, '1970-01-01'::timestamptz)
            )
        ) AS latest_span_ts
    FROM spans s
    WHERE s.trace_id IN (SELECT id FROM capped_traces)
    GROUP BY s.trace_id
),
event_activity AS (
    SELECT
        se.trace_id,
        MAX(
            GREATEST(
                COALESCE(se.event_ts, '1970-01-01'::timestamptz),
                se.server_ingested_at
            )
        ) AS latest_event_ts
    FROM span_events se
    WHERE se.trace_id IN (SELECT id FROM capped_traces)
    GROUP BY se.trace_id
)
SELECT
    ct.id,
    ct.trace_id,
    ct.name,
    ct.status,
    ct.user_id,
    ct.started_at,
    ct.ended_at,
    ct.metadata,
    ct.total_tokens_in,
    ct.total_tokens_out,
    ct.total_cost AS total_cost_usd,
    ct.error_count,
    GREATEST(
        ct.server_received_at,
        COALESCE(ct.ended_at, '1970-01-01'::timestamptz),
        COALESCE(sa.latest_span_ts, '1970-01-01'::timestamptz),
        COALESCE(ea.latest_event_ts, '1970-01-01'::timestamptz)
    )::timestamptz AS latest_activity_at,
    CASE
        WHEN ct.ended_at IS NULL THEN -1::bigint
        ELSE (EXTRACT(EPOCH FROM (ct.ended_at - ct.started_at)) * 1000)::bigint
    END AS duration_ms
FROM capped_traces ct
LEFT JOIN span_activity sa ON sa.trace_id = ct.id
LEFT JOIN event_activity ea ON ea.trace_id = ct.id
ORDER BY ct.started_at ASC, ct.id ASC;

-- name: ListSessionNarrativeSemanticEvents :many
SELECT
    sqlc.embed(se),
    s.name AS span_name
FROM span_events se
LEFT JOIN spans s
    ON s.trace_id = se.trace_id
   AND s.span_id = se.span_id
WHERE se.trace_id = ANY($1::uuid[])
  AND se.event_type IN ('decision', 'effect', 'wait')
ORDER BY COALESCE(se.event_ts, se.server_ingested_at) ASC, se.sequence ASC NULLS LAST;
