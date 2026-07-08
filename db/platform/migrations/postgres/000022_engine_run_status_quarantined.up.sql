ALTER TABLE traces
    DROP CONSTRAINT IF EXISTS traces_engine_run_status_check;

ALTER TABLE traces
    ADD CONSTRAINT traces_engine_run_status_check
    CHECK (
        engine_run_status IS NULL
        OR engine_run_status IN ('queued', 'running', 'waiting', 'suspended', 'quarantined', 'completed', 'failed', 'cancelled', 'terminated', 'continued_as_new')
    ) NOT VALID;

ALTER TABLE traces
    VALIDATE CONSTRAINT traces_engine_run_status_check;
