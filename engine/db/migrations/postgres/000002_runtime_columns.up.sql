ALTER TYPE engine.run_lifecycle_status ADD VALUE 'waiting';

ALTER TABLE engine.runs
    ADD COLUMN result JSONB,
    ADD COLUMN custom_status JSONB,
    ADD COLUMN waiting_for JSONB,
    ADD COLUMN completed_at TIMESTAMPTZ;
