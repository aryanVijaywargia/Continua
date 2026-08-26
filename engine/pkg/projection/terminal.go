package projection

import (
	"encoding/json"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	jsonraw "github.com/continua-ai/continua/engine/pkg/jsonraw"
)

// IsTerminalRunStatus reports whether an engine run has reached a state that
// accepts no further control operations. continued_as_new is terminal for the
// run that handed off, because its successor owns subsequent work.
func IsTerminalRunStatus(status enginedb.EngineRunLifecycleStatus) bool {
	switch status {
	case enginedb.EngineRunLifecycleStatusCompleted,
		enginedb.EngineRunLifecycleStatusFailed,
		enginedb.EngineRunLifecycleStatusCancelled,
		enginedb.EngineRunLifecycleStatusTerminated,
		enginedb.EngineRunLifecycleStatusContinuedAsNew:
		return true
	default:
		return false
	}
}

func TerminalStatuses(runStatus string) (traceStatus, spanStatus string) {
	switch runStatus {
	case "completed":
		return "completed", "completed"
	case "continued_as_new":
		return "completed", "completed"
	case "cancelled":
		return "cancelled", "failed"
	case "terminated":
		return "failed", "failed"
	default:
		return "failed", "failed"
	}
}

func TerminalOutputPayload(
	runStatus string,
	result json.RawMessage,
	errorCode *string,
	errorMessage *string,
) (json.RawMessage, error) {
	if runStatus == "completed" {
		return jsonraw.Clone(result), nil
	}
	if runStatus == "continued_as_new" {
		return nil, nil
	}
	return json.Marshal(map[string]any{
		"error_code":    derefString(errorCode),
		"error_message": derefString(errorMessage),
		"status":        runStatus,
	})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
