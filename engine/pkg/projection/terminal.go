package projection

import "encoding/json"

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
		return cloneRaw(result), nil
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

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
