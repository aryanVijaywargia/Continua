-- Convert search_vector from trigger-maintained to GENERATED columns
-- Phase 3: Spec 3 - Search & Filtering (D3 Decision compliance)
--
-- This migration converts the trigger-based search_vector columns to
-- PostgreSQL GENERATED ALWAYS AS STORED columns per the design spec.
--
-- Why generated columns (per design.md D3):
--   1. PostgreSQL 14+ guaranteed (user-confirmed minimum version)
--   2. Simpler than trigger-based approach
--   3. Automatic updates when source columns change
--   4. Better performance (computed at storage time)

-- Step 1: Drop existing triggers and functions
DROP TRIGGER IF EXISTS traces_search_vector_trigger ON traces;
DROP TRIGGER IF EXISTS spans_search_vector_trigger ON spans;
DROP FUNCTION IF EXISTS traces_search_vector_update();
DROP FUNCTION IF EXISTS spans_search_vector_update();

-- Step 2: Drop existing search_vector columns
ALTER TABLE traces DROP COLUMN IF EXISTS search_vector;
ALTER TABLE spans DROP COLUMN IF EXISTS search_vector;

-- Step 3: Add GENERATED columns for traces
ALTER TABLE traces ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(user_id, '')), 'B')
    ) STORED;

-- Step 4: Add GENERATED column for spans
ALTER TABLE spans ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('english', COALESCE(name, '')), 'A')
    ) STORED;

-- Step 5: Recreate GIN indexes for fast full-text search
CREATE INDEX IF NOT EXISTS idx_traces_search ON traces USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_spans_search ON spans USING GIN(search_vector);
