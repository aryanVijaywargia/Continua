ALTER TABLE traces
    ADD COLUMN engine_run_id UUID,
    ADD COLUMN engine_definition_name TEXT,
    ADD COLUMN engine_definition_version TEXT,
    ADD COLUMN engine_projection_state TEXT,
    ADD COLUMN engine_latest_history_id BIGINT,
    ADD COLUMN engine_last_projected_history_id BIGINT,
    ADD COLUMN engine_projection_updated_at TIMESTAMPTZ;

ALTER TABLE traces
    ADD CONSTRAINT traces_engine_projection_state_check
    CHECK (
        engine_projection_state IS NULL
        OR engine_projection_state IN ('up_to_date', 'catching_up', 'summary_only', 'journal_expired')
    );

CREATE UNIQUE INDEX idx_traces_engine_run_id_unique
    ON traces(engine_run_id)
    WHERE engine_run_id IS NOT NULL;

CREATE INDEX idx_traces_engine_projection_pending
    ON traces(engine_projection_state, engine_last_projected_history_id, engine_latest_history_id)
    WHERE engine_run_id IS NOT NULL;
