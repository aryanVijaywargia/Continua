-- name: ComputeTraceRollups :one
-- Compute rollup values for a trace by aggregating span data.
SELECT
    COUNT(*)::int AS total_spans,
    COALESCE(SUM(prompt_tokens), 0)::bigint AS total_tokens_in,
    COALESCE(SUM(completion_tokens), 0)::bigint AS total_tokens_out,
    COALESCE(SUM(total_cost), 0) AS total_cost,
    COUNT(*) FILTER (WHERE status IN ('failed', 'error'))::int AS error_count
FROM spans
WHERE trace_id = $1;
