DROP INDEX IF EXISTS idx_ingest_batch_payloads_created_at;

DROP TABLE IF EXISTS ingest_batch_payloads;

ALTER TABLE ingest_batches
    DROP COLUMN IF EXISTS last_error_at,
    DROP COLUMN IF EXISTS last_error_message,
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS processing_started_at;
