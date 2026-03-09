DROP INDEX IF EXISTS idx_ingest_batch_payloads_created_at;

CREATE INDEX IF NOT EXISTS idx_ingest_batches_failed_completed_at
    ON ingest_batches(processing_completed_at)
    WHERE status = 'failed';
