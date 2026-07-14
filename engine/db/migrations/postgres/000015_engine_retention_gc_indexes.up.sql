CREATE INDEX idx_engine_inbox_run_id
    ON engine.inbox (run_id)
    WHERE run_id IS NOT NULL;

CREATE INDEX idx_engine_request_dedupe_reap
    ON engine.request_dedupe (expires_at)
    WHERE status IN ('completed', 'failed', 'expired');
