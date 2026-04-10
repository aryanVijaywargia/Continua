ALTER TABLE engine.activity_tasks
    ADD COLUMN max_attempts INT NOT NULL DEFAULT 1,
    ADD COLUMN initial_backoff_ms BIGINT,
    ADD COLUMN max_backoff_ms BIGINT,
    ADD COLUMN backoff_multiplier DOUBLE PRECISION;
