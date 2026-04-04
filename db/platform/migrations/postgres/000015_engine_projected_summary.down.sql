ALTER TABLE traces
    DROP CONSTRAINT IF EXISTS traces_engine_run_status_check;

ALTER TABLE traces
    DROP COLUMN IF EXISTS engine_pending_inbox_items,
    DROP COLUMN IF EXISTS engine_pending_activity_tasks,
    DROP COLUMN IF EXISTS engine_wait_state,
    DROP COLUMN IF EXISTS engine_custom_status,
    DROP COLUMN IF EXISTS engine_run_status;
