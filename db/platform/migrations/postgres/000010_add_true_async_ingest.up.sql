ALTER TABLE ingest_batches
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error_code TEXT,
    ADD COLUMN IF NOT EXISTS last_error_message TEXT,
    ADD COLUMN IF NOT EXISTS last_error_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS ingest_batch_payloads (
    batch_id UUID PRIMARY KEY REFERENCES ingest_batches(id) ON DELETE CASCADE,
    payload_bytes BYTEA NOT NULL,
    compression TEXT NOT NULL DEFAULT 'gzip',
    content_type TEXT NOT NULL DEFAULT 'application/json',
    byte_size INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ingest_batch_payloads_created_at
    ON ingest_batch_payloads(created_at);
