-- Revert to trigger-based search_vector columns
-- This restores the previous trigger-based implementation

-- Step 1: Drop indexes and GENERATED columns
DROP INDEX IF EXISTS idx_traces_search;
DROP INDEX IF EXISTS idx_spans_search;
ALTER TABLE traces DROP COLUMN IF EXISTS search_vector;
ALTER TABLE spans DROP COLUMN IF EXISTS search_vector;

-- Step 2: Add regular columns
ALTER TABLE traces ADD COLUMN search_vector tsvector;
ALTER TABLE spans ADD COLUMN search_vector tsvector;

-- Step 3: Create trigger function for traces
CREATE OR REPLACE FUNCTION traces_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.user_id, '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Step 4: Create trigger for traces
CREATE TRIGGER traces_search_vector_trigger
    BEFORE INSERT OR UPDATE ON traces
    FOR EACH ROW
    EXECUTE FUNCTION traces_search_vector_update();

-- Step 5: Create trigger function for spans
CREATE OR REPLACE FUNCTION spans_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Step 6: Create trigger for spans
CREATE TRIGGER spans_search_vector_trigger
    BEFORE INSERT OR UPDATE ON spans
    FOR EACH ROW
    EXECUTE FUNCTION spans_search_vector_update();

-- Step 7: Recreate indexes
CREATE INDEX IF NOT EXISTS idx_traces_search ON traces USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_spans_search ON spans USING GIN(search_vector);

-- Step 8: Backfill existing rows
UPDATE traces SET search_vector =
    setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
    setweight(to_tsvector('english', COALESCE(user_id, '')), 'B');

UPDATE spans SET search_vector =
    setweight(to_tsvector('english', COALESCE(name, '')), 'A');
