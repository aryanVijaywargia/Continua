ALTER TABLE engine.definition_catalog
    ADD COLUMN runtime_published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;
