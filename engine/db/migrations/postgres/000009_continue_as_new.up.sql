ALTER TYPE engine.run_lifecycle_status ADD VALUE IF NOT EXISTS 'continued_as_new';

ALTER TABLE engine.runs
    ADD COLUMN continued_from_run_id UUID REFERENCES engine.runs(id),
    ADD COLUMN continued_to_run_id UUID REFERENCES engine.runs(id);

