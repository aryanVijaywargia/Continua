DO $$
DECLARE
    offending_tables TEXT[] := ARRAY[]::TEXT[];
BEGIN
    IF EXISTS (
        SELECT 1
        FROM engine.runs
        WHERE status = 'terminated'
    ) THEN
        offending_tables := array_append(offending_tables, 'engine.runs');
    END IF;

    IF EXISTS (
        SELECT 1
        FROM engine.instances
        WHERE status = 'terminated'
    ) THEN
        offending_tables := array_append(offending_tables, 'engine.instances');
    END IF;

    IF array_length(offending_tables, 1) IS NOT NULL THEN
        RAISE EXCEPTION 'cannot roll back terminated lifecycle enums while terminated rows still exist in: %',
            array_to_string(offending_tables, ', ');
    END IF;
END $$;

ALTER TYPE engine.run_lifecycle_status RENAME TO run_lifecycle_status_old;

CREATE TYPE engine.run_lifecycle_status AS ENUM (
    'queued',
    'running',
    'waiting',
    'completed',
    'failed',
    'cancelled'
);

ALTER TABLE engine.runs
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE engine.run_lifecycle_status
    USING status::text::engine.run_lifecycle_status;

DROP TYPE engine.run_lifecycle_status_old;

ALTER TABLE engine.runs
    ALTER COLUMN status SET DEFAULT 'queued';

ALTER TYPE engine.instance_lifecycle_status RENAME TO instance_lifecycle_status_old;

CREATE TYPE engine.instance_lifecycle_status AS ENUM (
    'active',
    'completed',
    'failed',
    'cancelled'
);

ALTER TABLE engine.instances
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE engine.instance_lifecycle_status
    USING status::text::engine.instance_lifecycle_status;

DROP TYPE engine.instance_lifecycle_status_old;

ALTER TABLE engine.instances
    ALTER COLUMN status SET DEFAULT 'active';
