CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_engine_inbox_run_id
    ON engine.inbox (run_id)
    WHERE run_id IS NOT NULL;
