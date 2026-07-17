package projection

// TraceEngineColumns is the projection schema contract for the engine-to-platform
// seam: the Writer in this package owns all engine writes into public.traces.
// Migrations 000013, 000014, 000015 (as altered by 000016), and 000021 own these
// columns. Any platform migration that adds, renames, or retypes an engine_*
// column on traces must update this manifest; schema_guard_test.go enforces the
// contract in both directions.
var TraceEngineColumns = map[string]string{
	"engine_run_id":                    "uuid",
	"engine_definition_name":           "text",
	"engine_definition_version":        "text",
	"engine_projection_state":          "text",
	"engine_latest_history_id":         "bigint",
	"engine_last_projected_history_id": "bigint",
	"engine_projection_updated_at":     "timestamp with time zone",
	"engine_instance_key":              "text",
	"engine_run_status":                "text",
	"engine_custom_status":             "jsonb",
	"engine_wait_state":                "jsonb",
	"engine_pending_activity_tasks":    "bigint",
	"engine_pending_inbox_items":       "bigint",
	"engine_parent_run_id":             "uuid",
	"engine_root_run_id":               "uuid",
	"engine_child_key":                 "text",
	"engine_child_depth":               "integer",
}
