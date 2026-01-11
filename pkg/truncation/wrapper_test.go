package truncation

import (
	"encoding/json"
	"testing"
)

func TestWrapInvalidJSON_EmptyData(t *testing.T) {
	result := WrapInvalidJSON(nil)
	if result.Data != nil {
		t.Errorf("expected nil data for empty input, got %v", result.Data)
	}
	if result.WasWrapped {
		t.Error("expected WasWrapped to be false for empty input")
	}
	if result.OriginalLen != 0 {
		t.Errorf("expected OriginalLen 0, got %d", result.OriginalLen)
	}

	result = WrapInvalidJSON([]byte{})
	if result.Data != nil {
		t.Errorf("expected nil data for empty slice, got %v", result.Data)
	}
}

func TestWrapInvalidJSON_ValidJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"object", `{"key": "value"}`},
		{"array", `[1, 2, 3]`},
		{"string", `"hello"`},
		{"number", `42`},
		{"boolean", `true`},
		{"null", `null`},
		{"nested", `{"nested": {"key": "value"}, "array": [1, 2]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.data)
			result := WrapInvalidJSON(data)

			if result.WasWrapped {
				t.Error("expected WasWrapped to be false for valid JSON")
			}
			if string(result.Data) != tt.data {
				t.Errorf("expected data unchanged, got %s", string(result.Data))
			}
			if result.OriginalLen != len(data) {
				t.Errorf("expected OriginalLen %d, got %d", len(data), result.OriginalLen)
			}
		})
	}
}

func TestWrapInvalidJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"plain text", "hello world"},
		{"partial json", `{"key": value}`},
		{"html", "<html>test</html>"},
		{"mixed", "prefix {json}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.data)
			result := WrapInvalidJSON(data)

			if !result.WasWrapped {
				t.Error("expected WasWrapped to be true for invalid JSON")
			}

			// Verify the result is valid JSON
			if !json.Valid(result.Data) {
				t.Errorf("expected valid JSON output, got %s", string(result.Data))
			}

			// Verify the structure uses spec-compliant field names
			var parsed map[string]string
			if err := json.Unmarshal(result.Data, &parsed); err != nil {
				t.Errorf("failed to parse wrapped result: %v", err)
			}
			if parsed["__continua_raw"] != tt.data {
				t.Errorf("expected __continua_raw field to be %q, got %q", tt.data, parsed["__continua_raw"])
			}
			if parsed["__parse_error"] == "" {
				t.Error("expected __parse_error field to be non-empty")
			}
			if result.ParseError == "" {
				t.Error("expected ParseError to be non-empty")
			}

			if result.OriginalLen != len(data) {
				t.Errorf("expected OriginalLen %d, got %d", len(data), result.OriginalLen)
			}
		})
	}
}

func TestEnsureJSON(t *testing.T) {
	// Valid JSON
	data, wrapped := EnsureJSON([]byte(`{"key": "value"}`))
	if wrapped {
		t.Error("expected wrapped to be false for valid JSON")
	}
	if string(data) != `{"key": "value"}` {
		t.Errorf("expected unchanged data, got %s", string(data))
	}

	// Invalid JSON
	data, wrapped = EnsureJSON([]byte("plain text"))
	if !wrapped {
		t.Error("expected wrapped to be true for invalid JSON")
	}
	if !json.Valid(data) {
		t.Errorf("expected valid JSON, got %s", string(data))
	}
}
