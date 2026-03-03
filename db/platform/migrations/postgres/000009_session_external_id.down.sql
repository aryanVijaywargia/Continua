-- Remove external_id from sessions
DROP INDEX IF EXISTS idx_sessions_project_external;
ALTER TABLE sessions DROP COLUMN external_id;
