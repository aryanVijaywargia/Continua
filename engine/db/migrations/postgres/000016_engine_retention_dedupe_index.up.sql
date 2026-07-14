CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_engine_request_dedupe_reap
    ON engine.request_dedupe (expires_at)
    WHERE status IN ('completed', 'failed', 'expired');
