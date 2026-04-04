-- name: GetCompareValidation :one
SELECT
    s.id AS session_id,
    s.external_id AS session_external_id,
    s.name AS session_name,
    baseline.id AS baseline_id,
    baseline.trace_id AS baseline_trace_id,
    baseline.name AS baseline_name,
    baseline.status AS baseline_status,
    baseline.user_id AS baseline_user_id,
    COALESCE(baseline.start_time, baseline.server_received_at) AS baseline_started_at,
    baseline.end_time AS baseline_ended_at,
    COALESCE(
        baseline.duration_ms,
        CASE
            WHEN baseline.end_time IS NULL THEN NULL
            ELSE (EXTRACT(EPOCH FROM (baseline.end_time - COALESCE(baseline.start_time, baseline.server_received_at))) * 1000)::bigint
        END
    ) AS baseline_duration_ms,
    COALESCE(baseline.error_count, 0::integer) AS baseline_error_count,
    baseline.total_cost AS baseline_total_cost_usd,
    baseline.total_tokens_in AS baseline_total_tokens_in,
    baseline.total_tokens_out AS baseline_total_tokens_out,
    baseline.engine_run_id AS baseline_engine_run_id,
    baseline.engine_definition_name AS baseline_engine_definition_name,
    baseline.engine_definition_version AS baseline_engine_definition_version,
    baseline.engine_projection_state AS baseline_engine_projection_state,
    candidate.id AS candidate_id,
    candidate.trace_id AS candidate_trace_id,
    candidate.name AS candidate_name,
    candidate.status AS candidate_status,
    candidate.user_id AS candidate_user_id,
    COALESCE(candidate.start_time, candidate.server_received_at) AS candidate_started_at,
    candidate.end_time AS candidate_ended_at,
    COALESCE(
        candidate.duration_ms,
        CASE
            WHEN candidate.end_time IS NULL THEN NULL
            ELSE (EXTRACT(EPOCH FROM (candidate.end_time - COALESCE(candidate.start_time, candidate.server_received_at))) * 1000)::bigint
        END
    ) AS candidate_duration_ms,
    COALESCE(candidate.error_count, 0::integer) AS candidate_error_count,
    candidate.total_cost AS candidate_total_cost_usd,
    candidate.total_tokens_in AS candidate_total_tokens_in,
    candidate.total_tokens_out AS candidate_total_tokens_out,
    candidate.engine_run_id AS candidate_engine_run_id,
    candidate.engine_definition_name AS candidate_engine_definition_name,
    candidate.engine_definition_version AS candidate_engine_definition_version,
    candidate.engine_projection_state AS candidate_engine_projection_state
FROM sessions s
JOIN traces baseline
    ON baseline.id = $3
   AND baseline.project_id = s.project_id
   AND baseline.session_id = s.id
JOIN traces candidate
    ON candidate.id = $4
   AND candidate.project_id = s.project_id
   AND candidate.session_id = s.id
WHERE s.id = $1
  AND s.project_id = $2;

-- name: GetCompareSpanCounts :many
SELECT
    trace_id,
    COUNT(*) AS span_count
FROM spans
WHERE trace_id = ANY($1::uuid[])
GROUP BY trace_id;

-- name: GetCompareSemanticCounts :many
SELECT
    trace_id,
    COUNT(*) AS semantic_count
FROM span_events
WHERE trace_id = ANY($1::uuid[])
  AND event_type IN ('decision', 'effect', 'wait')
GROUP BY trace_id;

-- name: ListCompareSpans :many
SELECT
    id,
    trace_id,
    span_id,
    parent_span_id,
    name,
    type,
    status,
    status_message,
    start_time,
    end_time,
    duration_ms,
    prompt_tokens,
    completion_tokens,
    total_cost,
    depth,
    server_received_at,
    sequence,
    model
FROM spans
WHERE trace_id = ANY($1::uuid[])
ORDER BY COALESCE(start_time, server_received_at) ASC, sequence NULLS LAST, id ASC;

-- name: ListCompareSemanticEvents :many
SELECT
    se.id,
    se.trace_id,
    se.span_id,
    se.event_type,
    se.event_ts,
    se.server_ingested_at,
    se.sequence,
    se.message,
    se.payload,
    s.name AS span_name
FROM span_events se
LEFT JOIN spans s
    ON s.trace_id = se.trace_id
   AND s.span_id = se.span_id
WHERE se.trace_id = ANY($1::uuid[])
  AND se.event_type IN ('decision', 'effect', 'wait')
ORDER BY COALESCE(se.event_ts, se.server_ingested_at) ASC, se.sequence ASC NULLS LAST, se.id ASC;
