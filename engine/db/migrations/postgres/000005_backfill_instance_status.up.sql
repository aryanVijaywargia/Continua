UPDATE engine.instances AS instances
SET status = latest.status
FROM (
    SELECT DISTINCT ON (runs.instance_id)
        runs.instance_id,
        CASE
            WHEN runs.status IN ('completed', 'failed', 'cancelled', 'terminated') THEN runs.status::text::engine.instance_lifecycle_status
            ELSE 'active'::engine.instance_lifecycle_status
        END AS status
    FROM engine.runs AS runs
    ORDER BY runs.instance_id, runs.created_at DESC, runs.id DESC
) AS latest
WHERE instances.id = latest.instance_id;
