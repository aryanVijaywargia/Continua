DO $$
DECLARE
    continued_as_new_count INTEGER;
    offending_rows TEXT;
    overflow_suffix TEXT := '';
BEGIN
    SELECT COUNT(*)
    INTO continued_as_new_count
    FROM engine.runs
    WHERE status::text = 'continued_as_new';

    IF continued_as_new_count > 0 THEN
        SELECT string_agg(format('id=%s instance_id=%s', id, instance_id), ', ' ORDER BY id)
        INTO offending_rows
        FROM (
            SELECT id, instance_id
            FROM engine.runs
            WHERE status::text = 'continued_as_new'
            ORDER BY id
            LIMIT 10
        ) blocked;

        IF continued_as_new_count > 10 THEN
            overflow_suffix := format(' (and %s more)', continued_as_new_count - 10);
        END IF;

        RAISE EXCEPTION '%', format(
            'cannot roll back continue-as-new lifecycle enum while engine.runs still contains continued_as_new rows: %s%s',
            offending_rows,
            overflow_suffix
        );
    END IF;
END $$;

ALTER TABLE engine.runs
    DROP COLUMN IF EXISTS continued_to_run_id,
    DROP COLUMN IF EXISTS continued_from_run_id;

CREATE TYPE engine.run_lifecycle_status_revert AS ENUM (
    'queued',
    'running',
    'waiting',
    'completed',
    'failed',
    'cancelled',
    'terminated',
    'suspended'
);

ALTER TABLE engine.runs
    ADD COLUMN status_revert engine.run_lifecycle_status_revert;

UPDATE engine.runs
SET status_revert = status::text::engine.run_lifecycle_status_revert;

ALTER TABLE engine.runs
    ALTER COLUMN status_revert SET NOT NULL,
    ALTER COLUMN status_revert SET DEFAULT 'queued';

ALTER TABLE engine.runs
    DROP COLUMN status;

DROP TYPE engine.run_lifecycle_status;

ALTER TABLE engine.runs
    RENAME COLUMN status_revert TO status;

ALTER TYPE engine.run_lifecycle_status_revert RENAME TO run_lifecycle_status;

