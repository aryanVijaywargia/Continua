-- name: GetEngineHealthMetrics :one
WITH engine_traces AS (
    SELECT t.engine_run_id,
           t.engine_projection_state,
           t.engine_latest_history_id,
           t.engine_last_projected_history_id
    FROM public.traces AS t
    WHERE t.engine_run_id IS NOT NULL
      AND (sqlc.narg(project_filter)::uuid IS NULL OR t.project_id = sqlc.narg(project_filter)::uuid)
), trace_lag AS (
    SELECT GREATEST(
               GREATEST(
                   COALESCE((
                       SELECT MAX(h.id)
                       FROM engine.history AS h
                       WHERE h.run_id = t.engine_run_id
                   ), 0),
                   COALESCE(t.engine_latest_history_id, 0)
               ) - COALESCE(t.engine_last_projected_history_id, 0),
               0
           ) AS lag_rows
    FROM engine_traces AS t
    WHERE COALESCE(t.engine_projection_state, '') NOT IN ('summary_only', 'journal_expired')
)
SELECT (
           SELECT COUNT(*)
           FROM engine.runs AS r
           WHERE (sqlc.narg(project_filter)::uuid IS NULL OR r.project_id = sqlc.narg(project_filter)::uuid)
             AND ((r.status = 'queued' AND r.ready_at <= NOW())
               OR (r.status = 'running' AND r.lease_expires_at IS NOT NULL AND r.lease_expires_at < NOW()))
       ) AS runs_ready,
       (
           SELECT COUNT(*)
           FROM engine.activity_tasks AS task
           WHERE (sqlc.narg(project_filter)::uuid IS NULL OR task.project_id = sqlc.narg(project_filter)::uuid)
             AND task.execution_target = 'local'
             AND ((task.status = 'queued' AND task.available_at <= NOW())
               OR (task.status = 'claimed' AND task.lease_expires_at IS NOT NULL AND task.lease_expires_at < NOW()))
       ) AS activity_tasks_pending,
       (
           SELECT COUNT(*)
           FROM engine.inbox AS inbox
           WHERE (sqlc.narg(project_filter)::uuid IS NULL OR inbox.project_id = sqlc.narg(project_filter)::uuid)
             AND inbox.status = 'pending'
             AND inbox.available_at <= NOW()
       ) AS inbox_pending,
       COALESCE((SELECT SUM(lag_rows) FROM trace_lag), 0)::bigint AS projector_lag_rows,
       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'catching_up') AS runs_catching_up,
       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'summary_only') AS summary_only_runs,
       (SELECT COUNT(*) FROM engine_traces WHERE engine_projection_state = 'journal_expired') AS journal_expired_runs;

-- name: ListEngineWorkerHealth :many
WITH worker_leases AS (
    SELECT COALESCE(r.claimed_by, '') AS id, r.claimed_at, r.lease_expires_at
    FROM engine.runs AS r
    WHERE r.claimed_by IS NOT NULL
      AND (sqlc.narg(project_filter)::uuid IS NULL OR r.project_id = sqlc.narg(project_filter)::uuid)
    UNION ALL
    SELECT COALESCE(task.claimed_by, '') AS id, task.claimed_at, task.lease_expires_at
    FROM engine.activity_tasks AS task
    WHERE task.claimed_by IS NOT NULL
      AND (sqlc.narg(project_filter)::uuid IS NULL OR task.project_id = sqlc.narg(project_filter)::uuid)
)
SELECT id,
       MAX(claimed_at)::timestamptz AS last_claim_at,
       COUNT(*) FILTER (WHERE lease_expires_at > NOW()) AS active_leases,
       COUNT(*) FILTER (WHERE lease_expires_at IS NOT NULL AND lease_expires_at <= NOW()) AS expired_leases,
       CASE
           WHEN COUNT(*) FILTER (WHERE lease_expires_at > NOW()) > 0 THEN 'active'
           ELSE 'stale'
       END AS status
FROM worker_leases
GROUP BY id
ORDER BY id;
