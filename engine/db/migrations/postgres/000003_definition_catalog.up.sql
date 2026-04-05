CREATE TABLE engine.definition_catalog (
    definition_name TEXT NOT NULL,
    definition_version TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (definition_name, definition_version)
);
