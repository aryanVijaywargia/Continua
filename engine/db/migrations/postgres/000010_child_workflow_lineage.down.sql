DROP INDEX IF EXISTS engine.idx_engine_runs_lineage;
DROP INDEX IF EXISTS engine.idx_engine_child_workflows_root_depth;
DROP INDEX IF EXISTS engine.idx_engine_child_workflows_current_child_run;
DROP INDEX IF EXISTS engine.idx_engine_child_workflows_parent_run;

DROP TABLE IF EXISTS engine.child_workflows;

ALTER TABLE engine.runs
    DROP CONSTRAINT IF EXISTS engine_runs_child_lineage_check,
    DROP CONSTRAINT IF EXISTS engine_runs_child_depth_check,
    DROP COLUMN IF EXISTS child_depth,
    DROP COLUMN IF EXISTS child_key,
    DROP COLUMN IF EXISTS root_run_id,
    DROP COLUMN IF EXISTS parent_run_id;

DROP TYPE IF EXISTS engine.child_workflow_status;
