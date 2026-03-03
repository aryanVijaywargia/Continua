-- Add external_id to sessions for SDK-friendly session keys
-- Use gen_random_uuid() as default for any existing rows to satisfy NOT NULL + unique
ALTER TABLE sessions ADD COLUMN external_id TEXT NOT NULL DEFAULT gen_random_uuid()::text;

-- Remove the default after backfill (new inserts must always provide external_id)
ALTER TABLE sessions ALTER COLUMN external_id DROP DEFAULT;

-- Add unique index for (project_id, external_id) lookups and upsert
CREATE UNIQUE INDEX idx_sessions_project_external ON sessions(project_id, external_id);
