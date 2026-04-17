ALTER TABLE engine.activity_tasks
    ADD COLUMN execution_target TEXT NOT NULL DEFAULT 'local',
    ADD COLUMN lease_duration_ms BIGINT,
    ADD CONSTRAINT engine_activity_tasks_execution_target_check
        CHECK (execution_target IN ('local', 'remote'));

CREATE INDEX idx_engine_activity_tasks_claim_local
    ON engine.activity_tasks(status, available_at, lease_expires_at)
    WHERE execution_target = 'local' AND status IN ('queued', 'claimed');

CREATE INDEX idx_engine_activity_tasks_claim_remote
    ON engine.activity_tasks(project_id, activity_type, status, available_at, lease_expires_at)
    WHERE execution_target = 'remote' AND status IN ('queued', 'claimed');
