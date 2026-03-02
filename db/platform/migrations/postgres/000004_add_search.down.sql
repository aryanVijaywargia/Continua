-- Revert full-text search capabilities
-- Phase 3: Spec 3 - Search & Filtering

-- Drop spans search infrastructure
DROP TRIGGER IF EXISTS spans_search_vector_trigger ON spans;
DROP FUNCTION IF EXISTS spans_search_vector_update();
DROP INDEX IF EXISTS idx_spans_search;
ALTER TABLE spans DROP COLUMN IF EXISTS search_vector;

-- Drop traces search infrastructure
DROP TRIGGER IF EXISTS traces_search_vector_trigger ON traces;
DROP FUNCTION IF EXISTS traces_search_vector_update();
DROP INDEX IF EXISTS idx_traces_search;
ALTER TABLE traces DROP COLUMN IF EXISTS search_vector;
