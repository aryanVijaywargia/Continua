-- Bump trace version on span insert/update for rollup coalescing
-- Phase 3: Async Rollups - ensures re-enqueue detects span-only updates
--
-- Problem: Rollup jobs use trace.version to detect if data changed during
-- processing. But span inserts don't bump trace version, so span-only updates
-- during a running rollup job are missed.
--
-- Solution: Add a trigger that increments trace.version whenever spans change.
-- This ensures the rollup worker's re-enqueue check works correctly.

-- Create function to bump trace version
CREATE OR REPLACE FUNCTION bump_trace_version_on_span_change() RETURNS trigger AS $$
BEGIN
    -- Bump version on the parent trace (use NEW.trace_id for insert/update)
    UPDATE traces
    SET version = version + 1, updated_at = NOW()
    WHERE id = NEW.trace_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on spans table
DROP TRIGGER IF EXISTS spans_bump_trace_version ON spans;
CREATE TRIGGER spans_bump_trace_version
    AFTER INSERT OR UPDATE ON spans
    FOR EACH ROW
    EXECUTE FUNCTION bump_trace_version_on_span_change();
