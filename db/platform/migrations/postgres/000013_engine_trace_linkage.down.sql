DROP INDEX IF EXISTS idx_traces_engine_projection_pending;
DROP INDEX IF EXISTS idx_traces_engine_run_id_unique;

ALTER TABLE traces
    DROP CONSTRAINT IF EXISTS traces_engine_projection_state_check;

ALTER TABLE traces
    DROP COLUMN IF EXISTS engine_projection_updated_at,
    DROP COLUMN IF EXISTS engine_last_projected_history_id,
    DROP COLUMN IF EXISTS engine_latest_history_id,
    DROP COLUMN IF EXISTS engine_projection_state,
    DROP COLUMN IF EXISTS engine_definition_version,
    DROP COLUMN IF EXISTS engine_definition_name,
    DROP COLUMN IF EXISTS engine_run_id;
