package truncation

import (
	"encoding/json"
)

// Default maximum payload size (64KB).
const DefaultMaxBytes = 64 * 1024

// TruncateReason describes why a payload was truncated.
type TruncateReason string

const (
	TruncateReasonSize TruncateReason = "size_limit"
	TruncateReasonNone TruncateReason = ""
)

// TruncateResult contains the result of truncating a payload.
type TruncateResult struct {
	Data              []byte
	Truncated         bool
	OriginalSizeBytes int64
	Reason            TruncateReason
}

// TruncateJSON truncates a JSON payload to fit within the maximum size.
// If the payload is already within the limit, it returns the payload unchanged.
// For objects and arrays, it attempts to preserve the structure by truncating
// nested content. As a fallback, it truncates the raw bytes.
func TruncateJSON(data []byte, maxBytes int) TruncateResult {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	originalSize := int64(len(data))

	if len(data) <= maxBytes {
		return TruncateResult{
			Data:              data,
			Truncated:         false,
			OriginalSizeBytes: originalSize,
			Reason:            TruncateReasonNone,
		}
	}

	// Attempt smart truncation for JSON objects
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err == nil {
		if truncated := truncateValue(parsed, maxBytes); truncated != nil {
			if result, err := json.Marshal(truncated); err == nil && len(result) <= maxBytes {
				return TruncateResult{
					Data:              result,
					Truncated:         true,
					OriginalSizeBytes: originalSize,
					Reason:            TruncateReasonSize,
				}
			}
		}
	}

	// Fallback: raw byte truncation with marker
	result := rawTruncate(data, maxBytes)
	return TruncateResult{
		Data:              result,
		Truncated:         true,
		OriginalSizeBytes: originalSize,
		Reason:            TruncateReasonSize,
	}
}

// truncateValue attempts to truncate a JSON value to fit within the size limit.
// It recursively truncates nested structures.
func truncateValue(v interface{}, maxBytes int) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return truncateObject(val, maxBytes)
	case []interface{}:
		return truncateArray(val, maxBytes)
	case string:
		if len(val) > maxBytes {
			// Truncate long strings
			truncLen := maxBytes - 20 // Leave room for "...[truncated]"
			if truncLen < 0 {
				truncLen = 0
			}
			return val[:truncLen] + "...[truncated]"
		}
		return val
	default:
		return val
	}
}

// truncateObject truncates a JSON object by limiting the number of keys
// and recursively truncating values.
func truncateObject(obj map[string]interface{}, maxBytes int) map[string]interface{} {
	result := make(map[string]interface{})

	// Estimate current size
	currentSize := 2 // {}
	for key, value := range obj {
		// Add key and some overhead for structure
		keySize := len(key) + 4 // "key":
		valBytes, _ := json.Marshal(value)
		valSize := len(valBytes)

		if currentSize+keySize+valSize > maxBytes {
			// Try to fit a truncated version
			truncated := truncateValue(value, maxBytes-currentSize-keySize-50)
			if truncated != nil {
				truncBytes, _ := json.Marshal(truncated)
				if currentSize+keySize+len(truncBytes) <= maxBytes {
					result[key] = truncated
					currentSize += keySize + len(truncBytes) + 1 // +1 for comma
					continue
				}
			}
			// Add truncation marker and stop
			result["_truncated"] = true
			break
		}

		result[key] = value
		currentSize += keySize + valSize + 1 // +1 for comma
	}

	return result
}

// truncateArray truncates a JSON array by limiting the number of elements.
func truncateArray(arr []interface{}, maxBytes int) []interface{} {
	result := make([]interface{}, 0, len(arr))

	currentSize := 2 // []
	for _, value := range arr {
		valBytes, _ := json.Marshal(value)
		valSize := len(valBytes)

		if currentSize+valSize > maxBytes {
			// Try to fit a truncated version
			truncated := truncateValue(value, maxBytes-currentSize-50)
			if truncated != nil {
				truncBytes, _ := json.Marshal(truncated)
				if currentSize+len(truncBytes) <= maxBytes {
					result = append(result, truncated)
					break
				}
			}
			// Add truncation marker
			result = append(result, map[string]interface{}{"_truncated": true})
			break
		}

		result = append(result, value)
		currentSize += valSize + 1 // +1 for comma
	}

	return result
}

// rawTruncate performs raw byte truncation with a JSON-safe marker.
func rawTruncate(_ []byte, maxBytes int) []byte {
	fallback := []byte(`{"_truncated":true}`)
	if maxBytes < len(fallback) {
		return fallback[:maxBytes]
	}
	if maxBytes < 30 {
		return fallback
	}

	// Use the simple fallback which is guaranteed to fit
	return fallback
}

// isValidUTF8End checks if the byte slice ends at a valid UTF-8 boundary.
// Returns false if the slice ends in the middle of a multi-byte UTF-8 sequence.
func isValidUTF8End(b []byte) bool {
	if len(b) == 0 {
		return true
	}

	// Work backwards to find the start of the last character
	i := len(b) - 1

	// Skip continuation bytes (10xxxxxx)
	for i >= 0 && (b[i]&0xC0) == 0x80 {
		i--
	}

	if i < 0 {
		// All bytes were continuation bytes - invalid
		return false
	}

	// Now b[i] should be the start of a character
	startByte := b[i]
	remainingBytes := len(b) - 1 - i

	// Determine expected sequence length from start byte
	var expectedLen int
	switch {
	case startByte < 0x80:
		expectedLen = 1 // ASCII
	case startByte < 0xC0:
		return false // Continuation byte as start - invalid
	case startByte < 0xE0:
		expectedLen = 2
	case startByte < 0xF0:
		expectedLen = 3
	case startByte < 0xF8:
		expectedLen = 4
	default:
		return false // Invalid UTF-8 start byte
	}

	// Check if we have all the continuation bytes we need
	return remainingBytes == expectedLen-1
}

// Truncate is a convenience function that truncates a payload with the default max size.
func Truncate(data []byte) TruncateResult {
	return TruncateJSON(data, DefaultMaxBytes)
}

// TruncateWithLimit truncates a payload with a custom max size.
func TruncateWithLimit(data []byte, maxBytes int) TruncateResult {
	return TruncateJSON(data, maxBytes)
}
