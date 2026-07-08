DO $$
DECLARE
    quarantined_count INTEGER;
    offending_rows TEXT;
    overflow_suffix TEXT := '';
BEGIN
    SELECT COUNT(*)
    INTO quarantined_count
    FROM traces
    WHERE engine_run_status = 'quarantined';

    IF quarantined_count > 0 THEN
        SELECT string_agg(format('id=%s trace_id=%s', id, trace_id), ', ' ORDER BY id)
        INTO offending_rows
        FROM (
            SELECT id, trace_id
            FROM traces
            WHERE engine_run_status = 'quarantined'
            ORDER BY id
            LIMIT 10
        ) blocked;

        IF quarantined_count > 10 THEN
            overflow_suffix := format(' (and %s more)', quarantined_count - 10);
        END IF;

        RAISE EXCEPTION '%', format(
            'cannot roll back traces_engine_run_status_check while traces.engine_run_status still contains quarantined rows: %s%s',
            offending_rows,
            overflow_suffix
        );
    END IF;
END $$;

ALTER TABLE traces
    DROP CONSTRAINT IF EXISTS traces_engine_run_status_check;

ALTER TABLE traces
    ADD CONSTRAINT traces_engine_run_status_check
    CHECK (
        engine_run_status IS NULL
        OR engine_run_status IN ('queued', 'running', 'waiting', 'suspended', 'completed', 'failed', 'cancelled', 'terminated', 'continued_as_new')
    ) NOT VALID;

ALTER TABLE traces
    VALIDATE CONSTRAINT traces_engine_run_status_check;
