-- Add split token columns to traces
ALTER TABLE traces ADD COLUMN total_tokens_in BIGINT NOT NULL DEFAULT 0;
ALTER TABLE traces ADD COLUMN total_tokens_out BIGINT NOT NULL DEFAULT 0;

-- Backfill from spans
UPDATE traces SET
    total_tokens_in = COALESCE((SELECT SUM(COALESCE(prompt_tokens, 0)) FROM spans WHERE spans.trace_id = traces.id), 0),
    total_tokens_out = COALESCE((SELECT SUM(COALESCE(completion_tokens, 0)) FROM spans WHERE spans.trace_id = traces.id), 0);

-- Drop the old column
ALTER TABLE traces DROP COLUMN total_tokens;
