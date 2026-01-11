package ingest

import (
	"encoding/json"

	"github.com/continua-ai/continua/pkg/truncation"
)

// processPayload processes an input payload, ensuring it's valid JSON
// and truncating if necessary.
//
//nolint:unparam // maxBytes is configurable for testing and future flexibility
func processPayload(data any, maxBytes int) (jsonData []byte, truncated bool, origSize *int64, reason *string) {
	if data == nil {
		return nil, false, nil, nil
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		// Wrap non-JSON serializable data
		wrapped := map[string]string{"error": "failed to serialize", "type": "unknown"}
		jsonBytes, _ = json.Marshal(wrapped)
	}

	// Ensure it's valid JSON (wrap if not)
	jsonBytes, _ = truncation.EnsureJSON(jsonBytes)

	// Truncate if necessary
	if maxBytes <= 0 {
		maxBytes = truncation.DefaultMaxBytes
	}

	result := truncation.TruncateWithLimit(jsonBytes, maxBytes)

	if !result.Truncated {
		return result.Data, false, nil, nil
	}

	reasonStr := string(result.Reason)
	return result.Data, true, &result.OriginalSizeBytes, &reasonStr
}

// defaultString returns the value if non-nil, otherwise returns the default.
func defaultString(val *string, def string) string {
	if val == nil {
		return def
	}
	return *val
}
