package api

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// traceToAPI converts a database trace to an API trace.
func traceToAPI(t *platform.Trace) Trace {
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

	// Token counts (map from total_tokens to tokens_in/out for backwards compat)
	if t.TotalTokens != nil {
		// Split roughly for now - proper tracking would separate these
		half := int(*t.TotalTokens / 2)
		trace.TotalTokensIn = &half
		remaining := int(*t.TotalTokens) - half
		trace.TotalTokensOut = &remaining
	}

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

	return trace
}

// spanToAPI converts a database span to an API span.
func spanToAPI(sp *platform.Span) Span {
	span := Span{
		Id:        sp.ID,
		TraceId:   sp.TraceID,
		Name:      sp.Name,
		Kind:      SpanKind(mapSpanKind(sp.Type)),
		Status:    SpanStatus(mapSpanStatus(sp.Status)),
		StartedAt: sp.StartTime,
	}

	// Parent span ID (convert from external string to UUID if possible)
	// For now, we don't have a direct mapping, so skip

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

	return span
}

// sessionToAPI converts a database session to an API session.
func sessionToAPI(s *platform.Session) Session {
	session := Session{
		Id:        s.ID,
		CreatedAt: s.CreatedAt,
	}

	if s.Name != nil {
		session.Name = s.Name
	}

	if len(s.Metadata) > 0 {
		var meta map[string]interface{}
		if err := parseJSON(s.Metadata, &meta); err == nil {
			session.Metadata = &meta
		}
	}

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
	case "completed":
		return "COMPLETED"
	case "failed", "error":
		return "FAILED"
	default:
		return "RUNNING"
	}
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

func parseJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
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

// Ensure time is used
var _ = time.Now
