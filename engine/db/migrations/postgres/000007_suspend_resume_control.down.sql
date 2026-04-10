DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM engine.runs
        WHERE status::text = 'suspended'
    ) THEN
        RAISE EXCEPTION 'cannot roll back suspended lifecycle enum while engine.runs still contains suspended rows';
    END IF;
END $$;

DROP INDEX IF EXISTS engine.idx_engine_runs_claim;

CREATE TYPE engine.run_lifecycle_status_revert AS ENUM (
    'queued',
    'running',
    'waiting',
    'completed',
    'failed',
    'cancelled',
    'terminated'
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

DO $$
BEGIN
    EXECUTE $sql$
        CREATE INDEX idx_engine_runs_claim
            ON engine.runs(status, ready_at, lease_expires_at)
            WHERE status IN ('queued', 'running')
    $sql$;
END $$;
