package api

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
)

// traceToAPI converts a database trace to an API trace.
func traceToAPI(t *store.TraceRead) Trace {
	trace := Trace{
		Id:     t.ID,
		Name:   deref(t.Name),
		Status: TraceStatus(mapTraceStatus(t.Status)),
	}

	// Session ID
	if t.SessionID.Valid {
		id := openapi_types.UUID(t.SessionID.Bytes)
		trace.SessionId = &id
	}
	if t.SessionExternalID != nil {
		trace.SessionExternalId = t.SessionExternalID
	}

	// Start time
	if t.StartTime.Valid {
		trace.StartedAt = t.StartTime.Time
	} else {
		trace.StartedAt = t.ServerReceivedAt
	}

	// End time
	if t.EndTime.Valid {
		trace.EndedAt = &t.EndTime.Time
	}

	// Token counts — mapped directly from DB split columns
	tokIn := int(t.TotalTokensIn)
	trace.TotalTokensIn = &tokIn
	tokOut := int(t.TotalTokensOut)
	trace.TotalTokensOut = &tokOut

	// Cost - pgtype.Numeric needs to be converted via string
	if t.TotalCost.Valid {
		if f, err := numericToFloat32(t.TotalCost); err == nil {
			trace.TotalCostUsd = &f
		}
	}

	// Metadata (returned as-is)
	if len(t.Metadata) > 0 {
		var meta map[string]interface{}
		// Parse JSON metadata
		if err := parseJSON(t.Metadata, &meta); err == nil {
			trace.Metadata = &meta
		}
	}

	// Error count (from rollups)
	if t.ErrorCount != nil {
		ec := int(*t.ErrorCount)
		trace.ErrorCount = &ec
	}

	return trace
}

// traceDetailToAPI converts a database trace to the detail API schema by
// composing the summary mapper output with debugger-specific fields.
// NOTE: oapi-codegen currently flattens TraceDetail's allOf shape, so any new
// summary fields added to traceToAPI must also be copied into TraceDetail here.
func traceDetailToAPI(t *store.TraceRead) TraceDetail {
	summary := traceToAPI(t)
	trace := TraceDetail{
		Id:                summary.Id,
		Name:              summary.Name,
		Status:            TraceDetailStatus(summary.Status),
		StartedAt:         summary.StartedAt,
		EndedAt:           summary.EndedAt,
		SessionId:         summary.SessionId,
		SessionExternalId: summary.SessionExternalId,
		TotalTokensIn:     summary.TotalTokensIn,
		TotalTokensOut:    summary.TotalTokensOut,
		TotalCostUsd:      summary.TotalCostUsd,
		ErrorCount:        summary.ErrorCount,
		Metadata:          summary.Metadata,
	}

	if t.TraceID != "" {
		trace.TraceId = &t.TraceID
	}
	if t.UserID != nil {
		trace.UserId = t.UserID
	}
	if len(t.Tags) > 0 {
		tags := append([]string(nil), t.Tags...)
		trace.Tags = &tags
	}
	if t.Environment != nil {
		trace.Environment = t.Environment
	}
	if t.Release != nil {
		trace.Release = t.Release
	}
	if input, ok := parseJSONValue(t.Input); ok {
		trace.Input = input
	}
	if output, ok := parseJSONValue(t.Output); ok {
		trace.Output = output
	}

	return trace
}

// spanToAPI converts a database span to an API span.
func spanToAPI(sp *platform.Span) Span {
	span := Span{
		Id:        sp.ID,
		TraceId:   sp.TraceID,
		SpanId:    sp.SpanID, // External span ID for tree building
		Name:      sp.Name,
		Kind:      SpanKind(mapSpanKind(sp.Type)),
		Status:    SpanStatus(mapSpanStatus(sp.Status)),
		StartedAt: sp.StartTime,
	}

	// Parent span ID - direct string copy from DB
	if sp.ParentSpanID != nil {
		span.ParentSpanId = sp.ParentSpanID
	}

	// End time
	if sp.EndTime.Valid {
		span.EndedAt = &sp.EndTime.Time
	}

	// Token counts
	if sp.PromptTokens != nil {
		ti := int(*sp.PromptTokens)
		span.TokensIn = &ti
	}
	if sp.CompletionTokens != nil {
		to := int(*sp.CompletionTokens)
		span.TokensOut = &to
	}

	// Cost - pgtype.Numeric needs to be converted via string
	if sp.TotalCost.Valid {
		if f, err := numericToFloat32(sp.TotalCost); err == nil {
			span.CostUsd = &f
		}
	}

	// Latency
	if sp.DurationMs != nil {
		latency := int(*sp.DurationMs)
		span.LatencyMs = &latency
	}

	// Error message
	if sp.StatusMessage != nil {
		span.ErrorMessage = sp.StatusMessage
	}

	// Metadata
	if len(sp.Metadata) > 0 {
		var meta map[string]interface{}
		if err := parseJSON(sp.Metadata, &meta); err == nil {
			span.Metadata = &meta
		}
	}

	// Input payload (JSON from DB bytes - can be any valid JSON)
	if input, ok := parseJSONValue(sp.Input); ok {
		span.Input = input
	}

	// Output payload (JSON from DB bytes - can be any valid JSON)
	if output, ok := parseJSONValue(sp.Output); ok {
		span.Output = output
	}

	if sp.Model != nil {
		span.Model = sp.Model
	}
	if sp.Provider != nil {
		span.Provider = sp.Provider
	}
	if sp.InputTruncated != nil {
		span.InputTruncated = sp.InputTruncated
	}
	if sp.InputOriginalSizeBytes != nil {
		span.InputOriginalSizeBytes = sp.InputOriginalSizeBytes
	}
	if sp.InputTruncationReason != nil {
		span.InputTruncationReason = sp.InputTruncationReason
	}
	if sp.OutputTruncated != nil {
		span.OutputTruncated = sp.OutputTruncated
	}
	if sp.OutputOriginalSizeBytes != nil {
		span.OutputOriginalSizeBytes = sp.OutputOriginalSizeBytes
	}
	if sp.OutputTruncationReason != nil {
		span.OutputTruncationReason = sp.OutputTruncationReason
	}

	return span
}

// explicitTimelineEventToAPI converts an explicit span event row to a timeline event.
func explicitTimelineEventToAPI(ev *platform.SpanEvent, spanName *string) TimelineEvent {
	event := TimelineEvent{
		EventType: mapExplicitTimelineEventType(ev.EventType),
		Id:        ev.ID.String(),
		Source:    Explicit,
		Timestamp: timelineEventDisplayTimestamp(ev.EventTs, ev.ServerIngestedAt),
		TraceId:   ev.TraceID,
	}

	spanID := ev.SpanID
	event.SpanId = &spanID

	if spanName != nil {
		event.SpanName = spanName
	}
	if level := mapTimelineEventLevel(ev.Level); level != nil {
		event.Level = level
	}
	if ev.Sequence != nil {
		event.Sequence = ev.Sequence
	}
	if ev.Message != nil {
		event.Message = ev.Message
	}
	if payload := parseJSONObject(ev.Payload); payload != nil {
		event.Payload = &payload
	}

	return event
}

// syntheticTimelineEventToAPI converts a span lifecycle marker to a timeline event.
func syntheticTimelineEventToAPI(sp *platform.Span, eventType TimelineEventType, timestamp time.Time) TimelineEvent {
	spanID := sp.SpanID
	spanName := sp.Name

	return TimelineEvent{
		EventType: eventType,
		Id:        syntheticTimelineEventID(sp.SpanID, eventType),
		Source:    Synthetic,
		SpanId:    &spanID,
		SpanName:  &spanName,
		Timestamp: timestamp,
		TraceId:   sp.TraceID,
	}
}

// sessionToAPI converts a database session to an API session.
func sessionToAPI(s *platform.Session) Session {
	session := Session{
		Id:         s.ID,
		ExternalId: s.ExternalID,
		CreatedAt:  s.CreatedAt,
	}

	if s.Name != nil {
		session.Name = s.Name
	}

	if s.UserID != nil {
		session.UserId = s.UserID
	}

	if len(s.Metadata) > 0 {
		var meta map[string]interface{}
		if err := parseJSON(s.Metadata, &meta); err == nil {
			session.Metadata = &meta
		}
	}

	return session
}

// sessionWithCountToAPI converts a session with trace count to an API session.
func sessionWithCountToAPI(s *platform.Session, traceCount int64) Session {
	session := sessionToAPI(s)
	tc := int(traceCount)
	session.TraceCount = &tc
	return session
}

// Helper functions

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mapTraceStatus(status string) string {
	switch status {
	case "running":
		return "RUNNING"
	case "completed", "ok":
		return "COMPLETED"
	case "failed", "error", "cancelled":
		return "FAILED"
	default:
		return "RUNNING"
	}
}

func mapTimelineTraceStatus(status string) TimelineResponseTraceStatus {
	return TimelineResponseTraceStatus(mapTraceStatus(status))
}

func mapSpanKind(spanType string) string {
	switch spanType {
	case "llm":
		return "LLM"
	case "tool":
		return "TOOL"
	case "chain":
		return "CHAIN"
	case "agent":
		return "AGENT"
	default:
		return "CUSTOM"
	}
}

func mapSpanStatus(status string) string {
	switch status {
	case "running":
		return "STARTED"
	case "completed":
		return "COMPLETED"
	case "failed", "error":
		return "FAILED"
	default:
		return "SCHEDULED"
	}
}

func mapExplicitTimelineEventType(eventType string) TimelineEventType {
	switch eventType {
	case "log":
		return TimelineEventTypeLog
	case "error":
		return TimelineEventTypeError
	case "exception":
		return TimelineEventTypeException
	case "message":
		return TimelineEventTypeMessage
	case "metric":
		return TimelineEventTypeMetric
	default:
		return TimelineEventTypeCustom
	}
}

func mapTimelineEventLevel(level string) *TimelineEventLevel {
	var mapped TimelineEventLevel

	switch level {
	case "debug":
		mapped = TimelineEventLevelDebug
	case "info":
		mapped = TimelineEventLevelInfo
	case "warning":
		mapped = TimelineEventLevelWarning
	case "error":
		mapped = TimelineEventLevelError
	default:
		return nil
	}

	return &mapped
}

func timelineEventDisplayTimestamp(eventTs pgtype.Timestamptz, fallback time.Time) time.Time {
	if eventTs.Valid {
		return eventTs.Time
	}
	return fallback
}

func parseJSONObject(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := parseJSON(data, &payload); err != nil {
		return nil
	}

	return payload
}

func syntheticTimelineEventID(spanID string, eventType TimelineEventType) string {
	return spanID + ":" + string(eventType)
}

func parseJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func parseJSONValue(data []byte) (interface{}, bool) {
	if len(data) == 0 {
		return nil, false
	}

	var value interface{}
	if err := parseJSON(data, &value); err != nil {
		return nil, false
	}
	if value == nil {
		return jsonNull{}, true
	}

	return value, true
}

type jsonNull struct{}

func (jsonNull) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// numericToFloat32 converts a pgtype.Numeric to float32.
func numericToFloat32(n pgtype.Numeric) (float32, error) {
	// Get the numeric as a float64 first
	f64, err := n.Float64Value()
	if err != nil {
		return 0, err
	}
	if !f64.Valid {
		return 0, nil
	}
	return float32(f64.Float64), nil
}
