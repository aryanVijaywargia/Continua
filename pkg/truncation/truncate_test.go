package truncation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTruncateJSON_SmallPayload(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	result := TruncateJSON(data, DefaultMaxBytes)

	if result.Truncated {
		t.Error("expected Truncated to be false for small payload")
	}
	if string(result.Data) != string(data) {
		t.Errorf("expected data unchanged, got %s", string(result.Data))
	}
	if result.OriginalSizeBytes != int64(len(data)) {
		t.Errorf("expected OriginalSizeBytes %d, got %d", len(data), result.OriginalSizeBytes)
	}
	if result.Reason != TruncateReasonNone {
		t.Errorf("expected no truncation reason, got %s", result.Reason)
	}
}

func TestTruncateJSON_LargePayload(t *testing.T) {
	// Create a payload larger than the limit
	largeValue := strings.Repeat("x", 1000)
	data := []byte(`{"key": "` + largeValue + `"}`)
	maxBytes := 100

	result := TruncateJSON(data, maxBytes)

	if !result.Truncated {
		t.Error("expected Truncated to be true for large payload")
	}
	if len(result.Data) > maxBytes {
		t.Errorf("expected data size <= %d, got %d", maxBytes, len(result.Data))
	}
	if result.OriginalSizeBytes != int64(len(data)) {
		t.Errorf("expected OriginalSizeBytes %d, got %d", len(data), result.OriginalSizeBytes)
	}
	if result.Reason != TruncateReasonSize {
		t.Errorf("expected truncation reason %s, got %s", TruncateReasonSize, result.Reason)
	}

	// Verify result is valid JSON
	if !json.Valid(result.Data) {
		t.Errorf("expected valid JSON output, got %s", string(result.Data))
	}
}

func TestTruncateJSON_LargeArray(t *testing.T) {
	// Create a large array
	elements := make([]int, 1000)
	for i := range elements {
		elements[i] = i
	}
	data, _ := json.Marshal(elements)
	maxBytes := 200

	result := TruncateJSON(data, maxBytes)

	if !result.Truncated {
		t.Error("expected Truncated to be true for large array")
	}
	if len(result.Data) > maxBytes {
		t.Errorf("expected data size <= %d, got %d", maxBytes, len(result.Data))
	}

	// Verify result is valid JSON
	if !json.Valid(result.Data) {
		t.Errorf("expected valid JSON output, got %s", string(result.Data))
	}
}

func TestTruncateJSON_NestedObject(t *testing.T) {
	// Create a nested object
	nested := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"data": strings.Repeat("x", 500),
			},
		},
		"other": "value",
	}
	data, _ := json.Marshal(nested)
	maxBytes := 100

	result := TruncateJSON(data, maxBytes)

	if !result.Truncated {
		t.Error("expected Truncated to be true for large nested object")
	}
	if len(result.Data) > maxBytes {
		t.Errorf("expected data size <= %d, got %d", maxBytes, len(result.Data))
	}

	// Verify result is valid JSON
	if !json.Valid(result.Data) {
		t.Errorf("expected valid JSON output, got %s", string(result.Data))
	}
}

func TestTruncateWithLimit(t *testing.T) {
	data := []byte(`{"key": "` + strings.Repeat("x", 100) + `"}`)

	// Should not truncate with larger limit
	result := TruncateWithLimit(data, 1000)
	if result.Truncated {
		t.Error("expected not truncated with large limit")
	}

	// Should truncate with smaller limit
	result = TruncateWithLimit(data, 50)
	if !result.Truncated {
		t.Error("expected truncated with small limit")
	}
}
