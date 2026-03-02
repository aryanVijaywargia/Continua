-- Revert trace version bump trigger

DROP TRIGGER IF EXISTS spans_bump_trace_version ON spans;
DROP FUNCTION IF EXISTS bump_trace_version_on_span_change();
