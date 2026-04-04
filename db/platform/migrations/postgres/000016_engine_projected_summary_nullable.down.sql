UPDATE traces
SET engine_pending_activity_tasks = COALESCE(engine_pending_activity_tasks, 0),
    engine_pending_inbox_items = COALESCE(engine_pending_inbox_items, 0)
WHERE engine_pending_activity_tasks IS NULL
   OR engine_pending_inbox_items IS NULL;

ALTER TABLE traces
    ALTER COLUMN engine_pending_activity_tasks SET DEFAULT 0,
    ALTER COLUMN engine_pending_activity_tasks SET NOT NULL,
    ALTER COLUMN engine_pending_inbox_items SET DEFAULT 0,
    ALTER COLUMN engine_pending_inbox_items SET NOT NULL;
