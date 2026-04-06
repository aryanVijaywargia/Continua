DO $$
DECLARE
    terminated_count INTEGER;
    offending_rows TEXT;
    overflow_suffix TEXT := '';
BEGIN
    SELECT COUNT(*)
    INTO terminated_count
    FROM traces
    WHERE engine_run_status = 'terminated';

    IF terminated_count > 0 THEN
        SELECT string_agg(format('id=%s trace_id=%s', id, trace_id), ', ' ORDER BY id)
        INTO offending_rows
        FROM (
            SELECT id, trace_id
            FROM traces
            WHERE engine_run_status = 'terminated'
            ORDER BY id
            LIMIT 10
        ) blocked;

        IF terminated_count > 10 THEN
            overflow_suffix := format(' (and %s more)', terminated_count - 10);
        END IF;

        RAISE EXCEPTION '%', format(
            'cannot roll back traces_engine_run_status_check while traces.engine_run_status still contains terminated rows: %s%s',
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
        OR engine_run_status IN ('queued', 'running', 'waiting', 'completed', 'failed', 'cancelled')
    );
