-- Add full-text search capabilities for traces and spans
-- Phase 3: Spec 3 - Search & Filtering
-- Note: Using regular columns with trigger instead of GENERATED ALWAYS AS
-- for better SQLC compatibility

-- Add search_vector column to traces table
ALTER TABLE traces ADD COLUMN IF NOT EXISTS search_vector tsvector;

-- Create function to update traces search vector
CREATE OR REPLACE FUNCTION traces_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.user_id, '')), 'B');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for traces search vector
DROP TRIGGER IF EXISTS traces_search_vector_trigger ON traces;
CREATE TRIGGER traces_search_vector_trigger
    BEFORE INSERT OR UPDATE ON traces
    FOR EACH ROW
    EXECUTE FUNCTION traces_search_vector_update();

-- Create GIN index for fast full-text search on traces
CREATE INDEX IF NOT EXISTS idx_traces_search ON traces USING GIN(search_vector);

-- Add search_vector column to spans table
ALTER TABLE spans ADD COLUMN IF NOT EXISTS search_vector tsvector;

-- Create function to update spans search vector
CREATE OR REPLACE FUNCTION spans_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.name, '')), 'A');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for spans search vector
DROP TRIGGER IF EXISTS spans_search_vector_trigger ON spans;
CREATE TRIGGER spans_search_vector_trigger
    BEFORE INSERT OR UPDATE ON spans
    FOR EACH ROW
    EXECUTE FUNCTION spans_search_vector_update();

-- Create GIN index for fast full-text search on spans
CREATE INDEX IF NOT EXISTS idx_spans_search ON spans USING GIN(search_vector);

-- Backfill existing rows
UPDATE traces SET search_vector =
    setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
    setweight(to_tsvector('english', COALESCE(user_id, '')), 'B');

UPDATE spans SET search_vector =
    setweight(to_tsvector('english', COALESCE(name, '')), 'A');
