-- Revert project-scoped indexes
-- Phase 3: Spec 4 - Query Performance

-- Drop project-scoped indexes
DROP INDEX IF EXISTS idx_traces_project_started_at;
DROP INDEX IF EXISTS idx_traces_project_server_received;

-- Restore original non-project-scoped indexes
CREATE INDEX idx_traces_started_at ON traces(start_time DESC NULLS LAST);
CREATE INDEX idx_traces_server_received ON traces(server_received_at DESC);

-- Update statistics
ANALYZE traces;
