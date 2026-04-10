ALTER TABLE engine.activity_tasks
    DROP COLUMN backoff_multiplier,
    DROP COLUMN max_backoff_ms,
    DROP COLUMN initial_backoff_ms,
    DROP COLUMN max_attempts;
