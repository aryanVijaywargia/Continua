package truncation

import (
	"encoding/json"
	"fmt"
)

// WrapResult contains the result of wrapping a payload.
type WrapResult struct {
	Data        []byte
	WasWrapped  bool
	OriginalLen int
	ParseError  string
}

// WrapInvalidJSON wraps non-JSON data in a JSON structure.
// If the data is already valid JSON, it returns the data unchanged.
// If the data is not valid JSON, it wraps it in:
// {"__continua_raw": "<escaped data>", "__parse_error": "<error message>"}
func WrapInvalidJSON(data []byte) WrapResult {
	if len(data) == 0 {
		return WrapResult{Data: nil, WasWrapped: false, OriginalLen: 0}
	}

	// Check if it's already valid JSON
	if json.Valid(data) {
		return WrapResult{Data: data, WasWrapped: false, OriginalLen: len(data)}
	}

	// Try to unmarshal to get a specific error message
	var parseError string
	var js interface{}
	if err := json.Unmarshal(data, &js); err != nil {
		parseError = err.Error()
	} else {
		parseError = "invalid JSON"
	}

	// Wrap in a JSON structure with spec-compliant field names
	wrapped := map[string]string{
		"__continua_raw": string(data),
		"__parse_error":  parseError,
	}
	result, err := json.Marshal(wrapped)
	if err != nil {
		// Fallback: wrap as hex if string encoding fails
		result = []byte(fmt.Sprintf(`{"__continua_raw_hex":"%x","__parse_error":"failed to encode as string"}`, data))
	}

	return WrapResult{Data: result, WasWrapped: true, OriginalLen: len(data), ParseError: parseError}
}

// EnsureJSON ensures the data is valid JSON, wrapping if necessary.
// Returns the JSON data and whether it was wrapped.
func EnsureJSON(data []byte) ([]byte, bool) {
	result := WrapInvalidJSON(data)
	return result.Data, result.WasWrapped
}
