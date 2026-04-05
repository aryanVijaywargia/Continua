ALTER TABLE traces
    ADD COLUMN engine_run_status TEXT,
    ADD COLUMN engine_custom_status JSONB,
    ADD COLUMN engine_wait_state JSONB,
    ADD COLUMN engine_pending_activity_tasks BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN engine_pending_inbox_items BIGINT NOT NULL DEFAULT 0;

UPDATE traces
SET engine_run_status = CASE
        WHEN status = 'completed' THEN 'completed'
        WHEN status = 'failed' THEN 'failed'
        WHEN status = 'cancelled' THEN 'cancelled'
        ELSE 'running'
    END
WHERE engine_run_id IS NOT NULL
  AND engine_run_status IS NULL;

ALTER TABLE traces
    ADD CONSTRAINT traces_engine_run_status_check
    CHECK (
        engine_run_status IS NULL
        OR engine_run_status IN ('queued', 'running', 'waiting', 'completed', 'failed', 'cancelled')
    );
