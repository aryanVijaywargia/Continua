CREATE TYPE engine.child_workflow_status AS ENUM (
    'active',
    'completed',
    'failed',
    'cancelled',
    'terminated'
);

ALTER TABLE engine.runs
    ADD COLUMN parent_run_id UUID REFERENCES engine.runs(id),
    ADD COLUMN root_run_id UUID REFERENCES engine.runs(id),
    ADD COLUMN child_key TEXT,
    ADD COLUMN child_depth INT;

UPDATE engine.runs
SET root_run_id = id,
    child_depth = 0
WHERE root_run_id IS NULL;

ALTER TABLE engine.runs
    ALTER COLUMN root_run_id SET NOT NULL,
    ALTER COLUMN child_depth SET NOT NULL,
    ADD CONSTRAINT engine_runs_child_depth_check CHECK (child_depth >= 0 AND child_depth <= 32),
    ADD CONSTRAINT engine_runs_child_lineage_check CHECK (
        (parent_run_id IS NULL AND child_key IS NULL AND child_depth = 0)
        OR (parent_run_id IS NOT NULL AND child_key IS NOT NULL AND child_depth > 0)
    );

CREATE TABLE engine.child_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    parent_instance_id UUID NOT NULL REFERENCES engine.instances(id),
    parent_run_id UUID NOT NULL REFERENCES engine.runs(id),
    child_key TEXT NOT NULL,
    requested_definition_name TEXT NOT NULL,
    requested_definition_version TEXT NOT NULL,
    child_instance_id UUID NOT NULL REFERENCES engine.instances(id),
    child_instance_key TEXT NOT NULL,
    current_child_run_id UUID NOT NULL REFERENCES engine.runs(id),
    terminal_child_run_id UUID REFERENCES engine.runs(id),
    root_run_id UUID NOT NULL REFERENCES engine.runs(id),
    child_depth INT NOT NULL CHECK (child_depth > 0 AND child_depth <= 32),
    continuation_count INT NOT NULL DEFAULT 0 CHECK (continuation_count >= 0),
    status engine.child_workflow_status NOT NULL DEFAULT 'active',
    parent_wait_failed_at TIMESTAMPTZ,
    parent_wait_error_code TEXT,
    parent_wait_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, parent_run_id, child_key),
    UNIQUE (project_id, child_instance_id)
);

CREATE INDEX idx_engine_child_workflows_parent_run
    ON engine.child_workflows(project_id, parent_run_id, child_key);

CREATE INDEX idx_engine_child_workflows_current_child_run
    ON engine.child_workflows(project_id, current_child_run_id)
    WHERE status = 'active';

CREATE INDEX idx_engine_child_workflows_root_depth
    ON engine.child_workflows(project_id, root_run_id, child_depth);

CREATE INDEX idx_engine_runs_lineage
    ON engine.runs(project_id, root_run_id, parent_run_id, child_depth);
