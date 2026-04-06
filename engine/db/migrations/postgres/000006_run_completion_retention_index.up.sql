CREATE INDEX idx_engine_runs_retention_completed_at
    ON engine.runs (completed_at, id)
    WHERE status IN ('completed', 'failed', 'cancelled', 'terminated');
