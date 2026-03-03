-- Restore the old total_tokens column
ALTER TABLE traces ADD COLUMN total_tokens BIGINT DEFAULT 0;

-- Backfill as sum of in + out
UPDATE traces SET total_tokens = total_tokens_in + total_tokens_out;

-- Drop the split columns
ALTER TABLE traces DROP COLUMN total_tokens_in;
ALTER TABLE traces DROP COLUMN total_tokens_out;
