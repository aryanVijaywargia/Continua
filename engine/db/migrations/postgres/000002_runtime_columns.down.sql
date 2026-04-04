DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM engine.runs
        WHERE status = 'waiting'
    ) THEN
        RAISE EXCEPTION 'cannot roll back engine runtime columns while engine.runs still contains waiting rows';
    END IF;
END $$;

ALTER TYPE engine.run_lifecycle_status RENAME TO run_lifecycle_status_old;

CREATE TYPE engine.run_lifecycle_status AS ENUM (
    'queued',
    'running',
    'completed',
    'failed',
    'cancelled'
);

ALTER TABLE engine.runs
    ALTER COLUMN status TYPE engine.run_lifecycle_status
    USING status::text::engine.run_lifecycle_status;

DROP TYPE engine.run_lifecycle_status_old;

ALTER TABLE engine.runs
    DROP COLUMN completed_at,
    DROP COLUMN waiting_for,
    DROP COLUMN custom_status,
    DROP COLUMN result;
