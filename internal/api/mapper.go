package api

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
)

const originalEventTypePayloadKey = "__continua_original_event_type"

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
	eventType, recognized := mapExplicitTimelineEventType(ev.EventType)
	event := TimelineEvent{
		EventType: eventType,
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
		if !recognized {
			payload = cloneTimelinePayloadWithOriginalType(payload, ev.EventType)
		}
		event.Payload = &payload
	} else if !recognized {
		payload := cloneTimelinePayloadWithOriginalType(nil, ev.EventType)
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
	switch store.NormalizeTraceStatus(status) {
	case store.TraceStatusBucketCompleted:
		return "COMPLETED"
	case store.TraceStatusBucketFailed:
		return "FAILED"
	default:
		return "RUNNING"
	}
}

type sessionNarrativeResponse struct {
	Summary sessionNarrativeSummary `json:"summary"`
	Traces  []SessionNarrativeTrace `json:"traces"`
}

type sessionCompareResponse struct {
	Session   CompareSessionHeader     `json:"session"`
	Baseline  CompareTraceHeader       `json:"baseline"`
	Candidate CompareTraceHeader       `json:"candidate"`
	Summary   CompareSummary           `json:"summary"`
	SpanDiffs []sessionCompareSpanDiff `json:"span_diffs"`
}

type sessionCompareSpanDiff struct {
	DiffStatus     CompareDiffStatus            `json:"diff_status"`
	MatchSource    *CompareMatchSource          `json:"match_source"`
	MatchReason    *string                      `json:"match_reason"`
	ChangedFields  []string                     `json:"changed_fields"`
	BaselineSpan   *CompareSpanSummary          `json:"baseline_span"`
	CandidateSpan  *CompareSpanSummary          `json:"candidate_span"`
	SemanticGroups []sessionCompareSemanticDiff `json:"semantic_groups"`
	Depth          int                          `json:"depth"`
}

type sessionCompareSemanticDiff struct {
	EventType      CompareSemanticEventType `json:"event_type"`
	DiffStatus     CompareDiffStatus        `json:"diff_status"`
	MatchSource    *CompareMatchSource      `json:"match_source"`
	MatchReason    *string                  `json:"match_reason"`
	ChangedFields  []string                 `json:"changed_fields"`
	BaselineEvent  *CompareSemanticSummary  `json:"baseline_event"`
	CandidateEvent *CompareSemanticSummary  `json:"candidate_event"`
}

type sessionNarrativeSummary struct {
	CompletedTraceCount int        `json:"completed_trace_count"`
	ExplicitLinkCount   int        `json:"explicit_link_count"`
	FailedTraceCount    int        `json:"failed_trace_count"`
	InferredLinkCount   int        `json:"inferred_link_count"`
	LastActivityAt      *time.Time `json:"last_activity_at"`
	ReturnedTraceCount  int        `json:"returned_trace_count"`
	RunningTraceCount   int        `json:"running_trace_count"`
	StartedAt           *time.Time `json:"started_at"`
	TotalCostUsd        float32    `json:"total_cost_usd"`
	TotalTokensIn       int64      `json:"total_tokens_in"`
	TotalTokensOut      int64      `json:"total_tokens_out"`
	TotalTraceCount     int        `json:"total_trace_count"`
	Truncated           bool       `json:"truncated"`
	UnlinkedTraceCount  int        `json:"unlinked_trace_count"`
}

func sessionNarrativeToAPI(narrative *store.SessionNarrative) sessionNarrativeResponse {
	traces := make([]SessionNarrativeTrace, len(narrative.Traces))
	for i := range narrative.Traces {
		traces[i] = sessionNarrativeTraceToAPI(&narrative.Traces[i])
	}

	return sessionNarrativeResponse{
		Summary: sessionNarrativeSummaryToAPI(&narrative.Summary),
		Traces:  traces,
	}
}

func sessionCompareToAPI(comparison *store.SessionComparison) sessionCompareResponse {
	spanDiffs := make([]sessionCompareSpanDiff, len(comparison.SpanDiffs))
	for i := range comparison.SpanDiffs {
		spanDiffs[i] = sessionCompareSpanDiffToAPI(&comparison.SpanDiffs[i])
	}

	return sessionCompareResponse{
		Session:   compareSessionHeaderToAPI(&comparison.Session),
		Baseline:  compareTraceHeaderToAPI(&comparison.Baseline),
		Candidate: compareTraceHeaderToAPI(&comparison.Candidate),
		Summary:   compareSummaryToAPI(&comparison.Summary),
		SpanDiffs: spanDiffs,
	}
}

func compareSessionHeaderToAPI(header *store.SessionCompareSessionHeader) CompareSessionHeader {
	return CompareSessionHeader{
		Id:         header.ID,
		ExternalId: header.ExternalID,
		Name:       header.Name,
	}
}

func compareTraceHeaderToAPI(header *store.SessionCompareTraceHeader) CompareTraceHeader {
	return CompareTraceHeader{
		Id:             header.ID,
		TraceId:        header.TraceID,
		Name:           header.Name,
		Status:         CompareTraceHeaderStatus(header.Status),
		UserId:         header.UserID,
		StartedAt:      header.StartedAt,
		EndedAt:        header.EndedAt,
		DurationMs:     header.DurationMs,
		ErrorCount:     header.ErrorCount,
		TotalCostUsd:   header.TotalCostUsd,
		TotalTokensIn:  header.TotalTokensIn,
		TotalTokensOut: header.TotalTokensOut,
	}
}

func compareSummaryToAPI(summary *store.SessionCompareSummary) CompareSummary {
	return CompareSummary{
		TotalSpansBaseline:      summary.TotalSpansBaseline,
		TotalSpansCandidate:     summary.TotalSpansCandidate,
		MatchedSpans:            summary.MatchedSpans,
		UnmatchedBaselineSpans:  summary.UnmatchedBaselineSpans,
		UnmatchedCandidateSpans: summary.UnmatchedCandidateSpans,
		HeuristicMatches:        summary.HeuristicMatches,
		DurationDeltaMs:         summary.DurationDeltaMs,
		TokensInDelta:           summary.TokensInDelta,
		TokensOutDelta:          summary.TokensOutDelta,
		CostDeltaUsd:            summary.CostDeltaUsd,
		TotalSemanticBaseline:   summary.TotalSemanticBaseline,
		TotalSemanticCandidate:  summary.TotalSemanticCandidate,
	}
}

func sessionCompareSpanDiffToAPI(row *store.SessionCompareSpanDiffRow) sessionCompareSpanDiff {
	semanticGroups := make([]sessionCompareSemanticDiff, len(row.SemanticGroups))
	for i := range row.SemanticGroups {
		semanticGroups[i] = sessionCompareSemanticDiffToAPI(&row.SemanticGroups[i])
	}

	return sessionCompareSpanDiff{
		DiffStatus:     CompareDiffStatus(row.DiffStatus),
		MatchSource:    compareMatchSourceToAPI(row.MatchSource),
		MatchReason:    row.MatchReason,
		ChangedFields:  append([]string(nil), row.ChangedFields...),
		BaselineSpan:   compareSpanSummaryToAPI(row.BaselineSpan),
		CandidateSpan:  compareSpanSummaryToAPI(row.CandidateSpan),
		SemanticGroups: semanticGroups,
		Depth:          row.Depth,
	}
}

func sessionCompareSemanticDiffToAPI(group *store.SessionCompareSemanticDiffGroup) sessionCompareSemanticDiff {
	return sessionCompareSemanticDiff{
		EventType:      CompareSemanticEventType(group.EventType),
		DiffStatus:     CompareDiffStatus(group.DiffStatus),
		MatchSource:    compareMatchSourceToAPI(group.MatchSource),
		MatchReason:    group.MatchReason,
		ChangedFields:  append([]string(nil), group.ChangedFields...),
		BaselineEvent:  compareSemanticSummaryToAPI(group.BaselineEvent),
		CandidateEvent: compareSemanticSummaryToAPI(group.CandidateEvent),
	}
}

func compareSpanSummaryToAPI(summary *store.SessionCompareSpanSummary) *CompareSpanSummary {
	if summary == nil {
		return nil
	}

	apiSummary := CompareSpanSummary{
		Id:           summary.ID,
		SpanId:       summary.SpanID,
		ParentSpanId: summary.ParentSpanID,
		Name:         summary.Name,
		Kind:         CompareSpanSummaryKind(summary.Kind),
		Status:       CompareSpanSummaryStatus(summary.Status),
		StartedAt:    summary.StartedAt,
		EndedAt:      summary.EndedAt,
		ErrorMessage: summary.ErrorMessage,
		Model:        summary.Model,
	}

	if summary.LatencyMs != nil {
		latencyMs := int(*summary.LatencyMs)
		apiSummary.LatencyMs = &latencyMs
	}
	if summary.TokensIn != nil {
		tokensIn := int(*summary.TokensIn)
		apiSummary.TokensIn = &tokensIn
	}
	if summary.TokensOut != nil {
		tokensOut := int(*summary.TokensOut)
		apiSummary.TokensOut = &tokensOut
	}
	if summary.CostUsd != nil {
		costUsd := *summary.CostUsd
		apiSummary.CostUsd = &costUsd
	}

	return &apiSummary
}

func compareSemanticSummaryToAPI(summary *store.SessionCompareSemanticSummary) *CompareSemanticSummary {
	if summary == nil {
		return nil
	}

	apiSummary := CompareSemanticSummary{
		Id:        summary.ID,
		SpanId:    summary.SpanID,
		SpanName:  summary.SpanName,
		EventType: CompareSemanticEventType(summary.EventType),
		Timestamp: summary.Timestamp,
		Message:   summary.Message,
	}

	if len(summary.Payload) > 0 {
		payload := make(map[string]interface{}, len(summary.Payload))
		for key, value := range summary.Payload {
			payload[key] = value
		}
		apiSummary.Payload = &payload
	}

	return &apiSummary
}

func compareMatchSourceToAPI(matchSource *store.SessionCompareMatchSource) *CompareMatchSource {
	if matchSource == nil {
		return nil
	}

	value := CompareMatchSource(*matchSource)
	return &value
}

func sessionNarrativeSummaryToAPI(summary *store.SessionNarrativeSummary) sessionNarrativeSummary {
	apiSummary := sessionNarrativeSummary{
		CompletedTraceCount: int(summary.CompletedTraceCount),
		ExplicitLinkCount:   int(summary.ExplicitLinkCount),
		FailedTraceCount:    int(summary.FailedTraceCount),
		InferredLinkCount:   int(summary.InferredLinkCount),
		ReturnedTraceCount:  int(summary.ReturnedTraceCount),
		RunningTraceCount:   int(summary.RunningTraceCount),
		StartedAt:           summary.StartedAt,
		LastActivityAt:      summary.LastActivityAt,
		TotalTokensIn:       summary.TotalTokensIn,
		TotalTokensOut:      summary.TotalTokensOut,
		TotalTraceCount:     int(summary.TotalTraceCount),
		Truncated:           summary.Truncated,
		UnlinkedTraceCount:  int(summary.UnlinkedTraceCount),
	}

	// Keep this mapper aligned with the summary query's status normalization semantics.
	// Summary last_activity_at is intentionally trace-level approximate; per-trace
	// latest_activity_at remains the authoritative activity timestamp.
	if totalCostUsd, err := numericToFloat32(summary.TotalCostUsd); err == nil {
		apiSummary.TotalCostUsd = totalCostUsd
	}

	return apiSummary
}

func sessionNarrativeTraceToAPI(trace *store.SessionNarrativeTrace) SessionNarrativeTrace {
	apiTrace := SessionNarrativeTrace{
		Id:               trace.ID,
		LatestActivityAt: trace.LatestActivityAt,
		Lineage:          sessionNarrativeLineageToAPI(&trace.Lineage),
		Name:             deref(trace.Name),
		SemanticEvents:   sessionNarrativeEventsToAPI(trace.SemanticEvents),
		StartedAt:        trace.StartedAt,
		Status:           SessionNarrativeTraceStatus(mapTraceStatus(trace.Status)),
		TraceId:          trace.TraceID,
		UserId:           trace.UserID,
	}

	totalTokensIn := trace.TotalTokensIn
	apiTrace.TotalTokensIn = &totalTokensIn

	totalTokensOut := trace.TotalTokensOut
	apiTrace.TotalTokensOut = &totalTokensOut

	errorCount := int(trace.ErrorCount)
	apiTrace.ErrorCount = &errorCount

	if trace.EndedAt != nil {
		apiTrace.EndedAt = trace.EndedAt
	}
	if trace.DurationMs != nil {
		apiTrace.DurationMs = trace.DurationMs
	}
	if totalCostUsd, err := numericToFloat32(trace.TotalCostUsd); err == nil {
		apiTrace.TotalCostUsd = &totalCostUsd
	}

	return apiTrace
}

func sessionNarrativeLineageToAPI(lineage *store.SessionNarrativeLineage) SessionNarrativeLineage {
	return SessionNarrativeLineage{
		LinkKind:      lineage.LinkKind,
		ParentTraceId: lineage.ParentTraceID,
		TriggerSpanId: lineage.TriggerSpanID,
		Type:          SessionNarrativeLineageType(lineage.Type),
	}
}

func sessionNarrativeEventsToAPI(events []store.SessionNarrativeSemanticEvent) []TimelineEvent {
	apiEvents := make([]TimelineEvent, len(events))
	for i := range events {
		apiEvents[i] = explicitTimelineEventToAPI(&events[i].SpanEvent, events[i].SpanName)
	}
	return apiEvents
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

func mapExplicitTimelineEventType(eventType string) (TimelineEventType, bool) {
	switch eventType {
	case "log":
		return TimelineEventTypeLog, true
	case "error":
		return TimelineEventTypeError, true
	case "exception":
		return TimelineEventTypeException, true
	case "message":
		return TimelineEventTypeMessage, true
	case "metric":
		return TimelineEventTypeMetric, true
	case "custom":
		return TimelineEventTypeCustom, true
	case "state_change":
		return TimelineEventTypeStateChange, true
	case "decision":
		return TimelineEventTypeDecision, true
	case "effect":
		return TimelineEventTypeEffect, true
	case "wait":
		return TimelineEventTypeWait, true
	default:
		return TimelineEventTypeCustom, false
	}
}

func cloneTimelinePayloadWithOriginalType(payload map[string]interface{}, originalType string) map[string]interface{} {
	cloned := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		cloned[key] = value
	}

	// TODO: consider pooling or lower-allocation path if this becomes hot
	cloned[originalEventTypePayloadKey] = originalType
	return cloned
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
