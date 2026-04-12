DROP INDEX IF EXISTS idx_traces_engine_child_depth_project;
DROP INDEX IF EXISTS idx_traces_engine_child_key_project;
DROP INDEX IF EXISTS idx_traces_engine_root_run_id_project;
DROP INDEX IF EXISTS idx_traces_engine_parent_run_id_project;
DROP INDEX IF EXISTS idx_traces_engine_definition_version_project;
DROP INDEX IF EXISTS idx_traces_engine_run_id_project;

ALTER TABLE traces
    DROP COLUMN IF EXISTS engine_child_depth,
    DROP COLUMN IF EXISTS engine_child_key,
    DROP COLUMN IF EXISTS engine_root_run_id,
    DROP COLUMN IF EXISTS engine_parent_run_id;
