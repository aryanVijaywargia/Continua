ALTER TABLE traces
    ALTER COLUMN engine_pending_activity_tasks DROP NOT NULL,
    ALTER COLUMN engine_pending_activity_tasks DROP DEFAULT,
    ALTER COLUMN engine_pending_inbox_items DROP NOT NULL,
    ALTER COLUMN engine_pending_inbox_items DROP DEFAULT;

UPDATE traces
SET engine_pending_activity_tasks = NULL,
    engine_pending_inbox_items = NULL
WHERE engine_run_id IS NULL;
