CREATE INDEX IF NOT EXISTS idx_sessions_project_created_id_desc
    ON sessions(project_id, created_at DESC, id DESC);
