package projection

// TraceEngineColumns declares the engine_* columns of public.traces that the
// projection Writer's SQL depends on, mapped to their expected
// information_schema.columns.data_type values.
var TraceEngineColumns = map[string]string{}
