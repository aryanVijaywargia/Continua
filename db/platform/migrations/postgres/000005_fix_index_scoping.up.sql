-- Fix index scoping for multi-tenant performance
-- Phase 3: Spec 4 - Query Performance

-- Drop non-project-scoped indexes that are used in project-filtered queries
DROP INDEX IF EXISTS idx_traces_started_at;
DROP INDEX IF EXISTS idx_traces_server_received;

-- Create project-scoped indexes for better multi-tenant query performance
-- These indexes have project_id first to enable efficient filtering in multi-tenant queries

-- Index for time-based ordering with project scope
CREATE INDEX idx_traces_project_started_at
    ON traces(project_id, start_time DESC NULLS LAST);

-- Index for server_received_at ordering with project scope
CREATE INDEX idx_traces_project_server_received
    ON traces(project_id, server_received_at DESC);

-- Update statistics for query planner
ANALYZE traces;
