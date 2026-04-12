ALTER TABLE traces
    ADD COLUMN engine_parent_run_id UUID,
    ADD COLUMN engine_root_run_id UUID,
    ADD COLUMN engine_child_key TEXT,
    ADD COLUMN engine_child_depth INTEGER;

UPDATE traces
SET engine_root_run_id = engine_run_id,
    engine_child_depth = 0
WHERE engine_run_id IS NOT NULL
  AND engine_root_run_id IS NULL;

CREATE INDEX idx_traces_engine_run_id_project
    ON traces(project_id, engine_run_id)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_definition_version_project
    ON traces(project_id, engine_definition_version)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_parent_run_id_project
    ON traces(project_id, engine_parent_run_id)
    WHERE engine_parent_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_root_run_id_project
    ON traces(project_id, engine_root_run_id)
    WHERE engine_root_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_child_key_project
    ON traces(project_id, engine_child_key)
    WHERE engine_child_key IS NOT NULL;

CREATE INDEX idx_traces_engine_child_depth_project
    ON traces(project_id, engine_child_depth)
    WHERE engine_child_depth IS NOT NULL;
