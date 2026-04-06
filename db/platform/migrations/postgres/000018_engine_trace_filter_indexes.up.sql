CREATE INDEX idx_traces_engine_instance_key_project
    ON traces(project_id, engine_instance_key)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_definition_name_project
    ON traces(project_id, engine_definition_name)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_run_status_project
    ON traces(project_id, engine_run_status)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_projection_state_project
    ON traces(project_id, engine_projection_state)
    WHERE engine_run_id IS NOT NULL;
