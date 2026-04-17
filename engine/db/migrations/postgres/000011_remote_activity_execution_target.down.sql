DROP INDEX IF EXISTS engine.idx_engine_activity_tasks_claim_remote;
DROP INDEX IF EXISTS engine.idx_engine_activity_tasks_claim_local;

ALTER TABLE engine.activity_tasks
    DROP CONSTRAINT IF EXISTS engine_activity_tasks_execution_target_check,
    DROP COLUMN IF EXISTS lease_duration_ms,
    DROP COLUMN IF EXISTS execution_target;
