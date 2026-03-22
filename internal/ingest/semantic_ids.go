package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/continua-ai/continua/pkg/truncation"
)

const (
	semanticIDNamespace = "continua-semantic-id"
	semanticIDVersion   = "v1"
	semanticIDSeparator = "\x1f"
)

type semanticFallbackContent struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Payload map[string]any `json:"payload"`
}

func deriveSemanticID(
	kind string,
	traceID string,
	spanID string,
	sequence *int32,
	eventTS *time.Time,
	level string,
	message string,
	payload map[string]any,
) string {
	parts := []string{
		semanticIDNamespace,
		semanticIDVersion,
		kind,
		traceID,
		spanID,
		formatSemanticSequence(sequence),
		formatSemanticEventTS(eventTS),
	}

	if sequence == nil && eventTS == nil {
		parts = append(parts, hashSemanticFallbackContent(level, message, payload))
	}

	return kind + "_" + hashSemanticString(strings.Join(parts, semanticIDSeparator))
}

func semanticPayloadIDKey(eventType string) string {
	switch eventType {
	case "effect":
		return "effect_id"
	case "wait":
		return "wait_id"
	default:
		return ""
	}
}

func semanticPayloadIDValue(eventType string, input *EventInput) string {
	key := semanticPayloadIDKey(eventType)
	if key == "" {
		return ""
	}

	if value, ok := payloadStringField(input.Payload, key); ok {
		return value
	}

	return deriveSemanticID(
		eventType,
		input.TraceID,
		input.SpanID,
		input.Sequence,
		input.EventTs,
		optionalString(input.Level),
		defaultString(input.Message, ""),
		input.Payload,
	)
}

func processEventPayload(eventType string, input *EventInput, maxBytes int) (jsonData []byte, truncated bool, origSize *int64, reason *string) {
	semanticKey := semanticPayloadIDKey(eventType)
	if semanticKey == "" {
		return processPayload(input.Payload, maxBytes)
	}

	semanticID := semanticPayloadIDValue(eventType, input)
	basePayload := cloneJSONObject(input.Payload)
	if basePayload == nil {
		basePayload = map[string]any{}
	}

	fullPayload := cloneJSONObject(basePayload)
	fullPayload[semanticKey] = semanticID

	fullJSONBytes := marshalPayloadData(fullPayload)
	if maxBytes <= 0 {
		maxBytes = truncation.DefaultMaxBytes
	}

	if len(fullJSONBytes) <= maxBytes {
		return fullJSONBytes, false, nil, nil
	}

	delete(basePayload, semanticKey)
	truncatedBytes := truncatePayloadWithReservedString(basePayload, semanticKey, semanticID, maxBytes)
	originalSize := int64(len(fullJSONBytes))
	reasonStr := string(truncation.TruncateReasonSize)

	return truncatedBytes, true, &originalSize, &reasonStr
}

func truncatePayloadWithReservedString(payload map[string]any, reservedKey, reservedValue string, maxBytes int) []byte {
	baseJSONBytes := marshalPayloadData(payload)
	if maxBytes <= 0 {
		maxBytes = truncation.DefaultMaxBytes
	}

	baseBudget := maxBytes - reservedStringJSONBudget(reservedKey, reservedValue)
	if baseBudget < 2 {
		baseBudget = 2
	}

	for {
		truncatedPayload := parsePayloadObject(truncation.TruncateWithLimit(baseJSONBytes, baseBudget).Data)
		truncatedPayload[reservedKey] = reservedValue

		finalJSONBytes := marshalPayloadData(truncatedPayload)
		if len(finalJSONBytes) <= maxBytes || baseBudget <= 2 {
			return finalJSONBytes
		}

		baseBudget--
	}
}

func reservedStringJSONBudget(key, value string) int {
	reservedOnlyJSON := marshalPayloadData(map[string]string{key: value})
	if len(reservedOnlyJSON) == 0 {
		return 0
	}

	return len(reservedOnlyJSON) - 1
}

func parsePayloadObject(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil || payload == nil {
		return map[string]any{}
	}

	return payload
}

func payloadStringField(payload map[string]any, field string) (string, bool) {
	if payload == nil {
		return "", false
	}

	value, ok := payload[field]
	if !ok {
		return "", false
	}

	text, ok := value.(string)
	return text, ok && text != ""
}

func cloneJSONObject(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}

	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = cloneJSONValue(value)
	}

	return cloned
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = cloneJSONValue(typed[i])
		}
		return cloned
	default:
		return typed
	}
}

func formatSemanticSequence(sequence *int32) string {
	if sequence == nil {
		return ""
	}

	return strconv.FormatInt(int64(*sequence), 10)
}

func formatSemanticEventTS(eventTS *time.Time) string {
	if eventTS == nil {
		return ""
	}

	return eventTS.UTC().Format(time.RFC3339Nano)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func hashSemanticFallbackContent(level, message string, payload map[string]any) string {
	content := semanticFallbackContent{
		Level:   level,
		Message: message,
		Payload: normalizeSemanticPayload(payload),
	}

	return hashSemanticBytes(marshalPayloadData(content))
}

func normalizeSemanticPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}

	normalized := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "effect_id" || key == "wait_id" || strings.HasPrefix(key, "__continua_") {
			continue
		}
		normalized[key] = normalizeSemanticValue(value)
	}

	return normalized
}

func normalizeSemanticValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, nestedValue := range typed {
			if strings.HasPrefix(key, "__continua_") {
				continue
			}
			normalized[key] = normalizeSemanticValue(nestedValue)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i := range typed {
			normalized[i] = normalizeSemanticValue(typed[i])
		}
		return normalized
	default:
		return typed
	}
}

func hashSemanticString(value string) string {
	return hashSemanticBytes([]byte(value))
}

func hashSemanticBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])[:32]
}
